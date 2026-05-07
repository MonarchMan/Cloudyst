package queue

import "time"

type TaskStatus string

const (
	StatusQueued     TaskStatus = "queued"
	StatusProcessing TaskStatus = "processing"
	StatusSuspending TaskStatus = "suspending"
	StatusError      TaskStatus = "error"
	StatusCanceled   TaskStatus = "canceled"
	StatusCompleted  TaskStatus = "completed"
)

const (
	QueuePrefix              = "queue_"
	WorkerNumSuffix          = "_worker_num"
	MaxExecutionSuffix       = "_max_execution"
	BackoffFactorSuffix      = "_backoff_factor"
	BackoffMaxDurationSuffix = "_backoff_max_duration"
	MaxRetrySuffix           = "_max_retry"
	RetryDelaySuffix         = "_retry_delay"
)

func (s TaskStatus) Values() []string {
	return []string{
		string(StatusQueued),
		string(StatusProcessing),
		string(StatusSuspending),
		string(StatusError),
		string(StatusCanceled),
		string(StatusCompleted),
	}
}

var StatusProtoValues = map[string]int32{
	string(StatusQueued):     1,
	string(StatusProcessing): 2,
	string(StatusSuspending): 3,
	string(StatusError):      4,
	string(StatusCanceled):   5,
	string(StatusCompleted):  6,
}

var ProtoToStatusValues = map[int32]TaskStatus{
	1: StatusQueued,
	2: StatusProcessing,
	3: StatusSuspending,
	4: StatusError,
	5: StatusCanceled,
	6: StatusCompleted,
}

type TaskPublicState struct {
	Error            string          `json:"error,omitempty"`
	ErrorHistory     []string        `json:"error_history,omitempty"`
	ExecutedDuration time.Duration   `json:"executed_duration,omitempty"`
	RetryCount       int             `json:"retry_count,omitempty"`
	ResumeTime       int64           `json:"resume_time,omitempty"`
	SlaveTaskProps   *SlaveTaskProps `json:"slave_task_props,omitempty"`
}

type SlaveTaskProps struct {
	NodeID            int    `json:"node_id,omitempty"`
	MasterSiteURl     string `json:"master_site_u_rl,omitempty"`
	MasterSiteID      string `json:"master_site_id,omitempty"`
	MasterSiteVersion string `json:"master_site_version,omitempty"`
}

type TaskArgs struct {
	Status       TaskStatus
	Type         string
	PublicState  *TaskPublicState
	PrivateState string
	OwnerID      int
	TraceID      string
}

// QueueSetting queue setting
type QueueSetting struct {
	WorkerNum          int
	MaxExecution       time.Duration
	BackoffFactor      float64
	BackoffMaxDuration time.Duration
	MaxRetry           int
	RetryDelay         time.Duration
}
