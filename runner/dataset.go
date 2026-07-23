package runner

// Dataset supplies serve batches and one sequential train epoch over the 80% split.
type Dataset interface {
	NextServe(phase string) Sample
	// ResetEpoch prepares a full pass over train data starting at sample offset
	// (for resume mid-epoch). offset is in examples, not batches.
	ResetEpoch(offset int)
	// NextTrain returns the next micro-batch. ok=false when the epoch is finished.
	NextTrain() (s Sample, ok bool)
	// TrainLen is the number of train examples in one epoch (e.g. 48000).
	TrainLen() int
	// EpochOffset is how many train examples have been consumed this epoch.
	EpochOffset() int
}
