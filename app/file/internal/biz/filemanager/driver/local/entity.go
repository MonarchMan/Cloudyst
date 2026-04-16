package local

import (
	"common/util"
	"file/ent"
	"file/internal/biz/filemanager/fs"
	"file/internal/data/types"
	"os"
	"time"

	"github.com/gofrs/uuid"
)

// NewLocalFileEntity creates a new local files entity.
func NewLocalFileEntity(t int, src string) (fs.Entity, error) {
	info, err := os.Stat(util.RelativePath(src))
	if err != nil {
		return nil, err
	}

	return &localFileEntity{
		t:    t,
		src:  src,
		size: info.Size(),
	}, nil
}

type localFileEntity struct {
	t    int
	src  string
	size int64
}

func (l *localFileEntity) ID() int {
	return 0
}

func (l *localFileEntity) Type() int {
	return l.t
}

func (l *localFileEntity) Size() int64 {
	return l.size
}

func (l *localFileEntity) UpdatedAt() time.Time {
	return time.Now()
}

func (l *localFileEntity) CreatedAt() time.Time {
	return time.Now()
}

func (l *localFileEntity) CreatedBy() int {
	return 0
}

func (l *localFileEntity) Source() string {
	return l.src
}

func (l *localFileEntity) ReferenceCount() int {
	return 1
}

func (l *localFileEntity) PolicyID() int {
	return 0
}

func (l *localFileEntity) UploadSessionID() *uuid.UUID {
	return nil
}

func (l *localFileEntity) Model() *ent.Entity {
	return nil
}

func (l *localFileEntity) Props() *types.EntityProps {
	return nil
}

func (l *localFileEntity) Encrypted() bool {
	return false
}
