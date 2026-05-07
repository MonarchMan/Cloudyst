package ingestion

import "fmt"

type ingestOptions struct {
	checkPointID string
	resumeData   map[string]any
	forceNewRun  bool
}

type IngestOption func(*ingestOptions)

const IngestCheckPointPrefix = "rag_ingest_document:"

func IngestCheckPointID(docID int) string {
	return fmt.Sprintf("%s%d", IngestCheckPointPrefix, docID)
}

func WithIngestCheckPointID(checkPointID string) IngestOption {
	return func(o *ingestOptions) {
		o.checkPointID = checkPointID
	}
}

func WithIngestResume(interruptIDs ...string) IngestOption {
	return func(o *ingestOptions) {
		if o.resumeData == nil {
			o.resumeData = make(map[string]any, len(interruptIDs))
		}
		for _, interruptID := range interruptIDs {
			o.resumeData[interruptID] = nil
		}
	}
}

func WithIngestResumeData(resumeData map[string]any) IngestOption {
	return func(o *ingestOptions) {
		if len(resumeData) == 0 {
			return
		}
		if o.resumeData == nil {
			o.resumeData = make(map[string]any, len(resumeData))
		}
		for interruptID, data := range resumeData {
			o.resumeData[interruptID] = data
		}
	}
}

func WithIngestForceNewRun() IngestOption {
	return func(o *ingestOptions) {
		o.forceNewRun = true
	}
}
