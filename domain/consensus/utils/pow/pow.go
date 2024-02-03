package pow

import (
	"github.com/karlsen-network/karlsend/cmd/karlsenminer/custoption"
	"github.com/karlsen-network/karlsend/domain/consensus/model/externalapi"
	"github.com/karlsen-network/karlsend/domain/consensus/utils/consensushashing"
	"github.com/karlsen-network/karlsend/domain/consensus/utils/hashes"
	"github.com/karlsen-network/karlsend/domain/consensus/utils/serialization"
	"github.com/karlsen-network/karlsend/infrastructure/logger"
	"github.com/karlsen-network/karlsend/util/difficulty"
	"github.com/karlsen-network/karlsend/util/panics"
	"github.com/pkg/errors"

	"math/big"
)

const hashingAlgoVersion = "fishhash-kls-0.0.1"

// State is an intermediate data structure with pre-computed values to speed up mining.
type State struct {
	mat        matrix
	Timestamp  int64
	Nonce      uint64
	Target     big.Int
	prePowHash externalapi.DomainHash
	//cache 	   cache
	context fishhashContext
}

// var context *fishhashContext
var sharedContext *fishhashContext

func getContext(full bool, log *logger.Logger, customOpt *custoption.Option) *fishhashContext {
	sharedContextLock.Lock()
	defer sharedContextLock.Unlock()

	if sharedContext != nil {
		if !full || sharedContext.FullDataset != nil {
			log.Debugf("log0 getContext ====")
			return sharedContext
		}
		log.Debugf("log1 getContext ==== going to build dataset")
	}

	// DISABLE LIGHT CACHE FOR THE MOMENT

	lightCache := make([]*hash512, lightCacheNumItems)
	log.Infof("Building light cache")
	buildLightCache(lightCache, lightCacheNumItems, seed)

	log.Debugf("getContext object 0 : %x", lightCache[0])
	log.Debugf("getContext object 42 : %x", lightCache[42])
	log.Debugf("getContext object 100 : %x", lightCache[100])

	fullDataset := make([]hash1024, fullDatasetNumItems)

	sharedContext = &fishhashContext{
		ready:               false,
		LightCacheNumItems:  lightCacheNumItems,
		LightCache:          lightCache,
		FullDatasetNumItems: fullDatasetNumItems,
		FullDataset:         fullDataset,
	}

	if full {
		//TODO : we forced the threads to 8 - must be calculated and parameterized
		prebuildDataset(sharedContext, 7, customOpt)
	} else {
		log.Infof("Dataset building SKIPPED - we must be on node")
	}

	return sharedContext
}

// SetLogger uses a specified Logger to output package logging info
func SetLogger(backend *logger.Backend, level logger.Level) {
	const logSubsystem = "POW"
	log = backend.Logger(logSubsystem)
	log.SetLevel(level)
	spawn = panics.GoroutineWrapperFunc(log)
}

// NewState creates a new state with pre-computed values to speed up mining
// It takes the target from the Bits field
func NewState(header externalapi.MutableBlockHeader, generatedag bool, customOpt *custoption.Option) *State {
	target := difficulty.CompactToBig(header.Bits())
	// Zero out the time and nonce.
	timestamp, nonce := header.TimeInMilliseconds(), header.Nonce()
	header.SetTimeInMilliseconds(0)
	header.SetNonce(0)
	prePowHash := consensushashing.HeaderHash(header)
	header.SetTimeInMilliseconds(timestamp)
	header.SetNonce(nonce)

	return &State{
		Target:     *target,
		prePowHash: *prePowHash,
		//will remove matrix opow
		//mat:       *generateMatrix(prePowHash),
		Timestamp: timestamp,
		Nonce:     nonce,
		context:   *getContext(generatedag, log, customOpt),
	}
}

func GetHashingAlgoVersion() string {
	return hashingAlgoVersion
}

func (state *State) IsContextReady() bool {
	if state != nil && &state.context != nil {
		return state.context.ready
	} else {
		return false
	}
}

// CalculateProofOfWorkValue hashes the internal header and returns its big.Int value
func (state *State) CalculateProofOfWorkValue() *big.Int {
	// PRE_POW_HASH || TIME || 32 zero byte padding || NONCE
	writer := hashes.NewPoWHashWriter()
	writer.InfallibleWrite(state.prePowHash.ByteSlice())
	err := serialization.WriteElement(writer, state.Timestamp)
	if err != nil {
		panic(errors.Wrap(err, "this should never happen. Hash digest should never return an error"))
	}

	zeroes := [32]byte{}
	writer.InfallibleWrite(zeroes[:])
	err = serialization.WriteElement(writer, state.Nonce)
	if err != nil {
		panic(errors.Wrap(err, "this should never happen. Hash digest should never return an error"))
	}
	//log.Debugf("Hash prePowHash %x\n", state.prePowHash.ByteSlice())
	//fmt.Printf("Hash prePowHash %x\n", state.prePowHash.ByteSlice())
	powHash := writer.Finalize()
	//middleHash := state.mat.HeavyHash(powHash)
	//log.Debugf("Hash b3-1: %x\n", powHash.ByteSlice())
	//fmt.Printf("Hash b3-1: %x\n", powHash.ByteSlice())
	middleHash := fishHash(&state.context, powHash)
	//log.Debugf("Hash fish: %x\n", middleHash.ByteSlice())
	//fmt.Printf("Hash fish: %x\n", middleHash.ByteSlice())

	writer2 := hashes.NewPoWHashWriter()
	writer2.InfallibleWrite(middleHash.ByteSlice())
	finalHash := writer2.Finalize()

	//log.Debugf("Hash b3-2: %x\n", finalHash.ByteSlice())

	return toBig(finalHash)
}

// IncrementNonce the nonce in State by 1
func (state *State) IncrementNonce() {
	state.Nonce++
}

// CheckProofOfWork check's if the block has a valid PoW according to the provided target
// it does not check if the difficulty itself is valid or less than the maximum for the appropriate network
func (state *State) CheckProofOfWork() bool {
	// The block pow must be less than the claimed target
	powNum := state.CalculateProofOfWorkValue()

	// The block hash must be less or equal than the claimed target.
	return powNum.Cmp(&state.Target) <= 0
}

// CheckProofOfWorkByBits check's if the block has a valid PoW according to its Bits field
// it does not check if the difficulty itself is valid or less than the maximum for the appropriate network
func CheckProofOfWorkByBits(header externalapi.MutableBlockHeader) bool {
	return NewState(header, false, nil).CheckProofOfWork()
}

// ToBig converts a externalapi.DomainHash into a big.Int treated as a little endian string.
func toBig(hash *externalapi.DomainHash) *big.Int {
	// We treat the Hash as little-endian for PoW purposes, but the big package wants the bytes in big-endian, so reverse them.
	buf := hash.ByteSlice()
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}

	return new(big.Int).SetBytes(buf)
}

// BlockLevel returns the block level of the given header.
func BlockLevel(header externalapi.BlockHeader, maxBlockLevel int) int {
	// Genesis is defined to be the root of all blocks at all levels, so we define it to be the maximal
	// block level.
	if len(header.DirectParents()) == 0 {
		return maxBlockLevel
	}

	proofOfWorkValue := NewState(header.ToMutable(), false, nil).CalculateProofOfWorkValue()
	level := maxBlockLevel - proofOfWorkValue.BitLen()
	// If the block has a level lower than genesis make it zero.
	if level < 0 {
		level = 0
	}
	return level
}
