package indexer

import (
	pbai "api/api/ai/admin/v1"
	"context"
	searcher "file/internal/biz/searher"
	"file/internal/data/rpc"
)

const ()

type RemoteIndexer struct {
	client pbai.AdminClient
}

func (r *RemoteIndexer) IndexFile(ctx context.Context, knowledgeID int, fileName, uri string) error {
	_, err := rpc.CreateDocument(ctx, r.client, knowledgeID, fileName, uri)
	return err
}

func (r *RemoteIndexer) DeleteByFileIDs(ctx context.Context, uris ...string) error {

}

func (r *RemoteIndexer) ChangeOwner(ctx context.Context, fileID, oldOwnerID, newOwnerID int) error {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) CopyByFileID(ctx context.Context, srcFileID, dstFileID, dstOwnerID, dstEntityID int) error {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) Rename(ctx context.Context, fileID, entityID int, newFileName string) error {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) Search(ctx context.Context, ownerID int, query string, offset int) ([]searcher.SearchResult, int64, error) {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) IndexReady(ctx context.Context) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) EnsureIndex(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) DeleteAll(ctx context.Context) error {
	//TODO implement me
	panic("implement me")
}

func (r *RemoteIndexer) Close() error {
	//TODO implement me
	panic("implement me")
}
