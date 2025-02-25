package job

import (
	"github.com/4aleksei/gmart/internal/common/store"
)

type (
	JobID  uint64
	Result struct {
		ID      JobID
		Result  int
		WaitSec int
		Value   store.Order
		Err     error
	}

	Job struct {
		ID    JobID
		Value store.Order
	}
	JobDone struct{}
)
