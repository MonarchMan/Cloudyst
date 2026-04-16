package knowledge

import (
	"ai/ent"
	"ai/internal/biz/knowledge/rag/ingestion"
	"ai/internal/biz/knowledge/rag/retrieval"
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/data/rpc"
	"ai/internal/data/vector"
	"ai/internal/pkg/eino/doc/rerank"
	"ai/internal/pkg/utils"
	"api/external/trans"
	"common/boolset"
	"common/constants"
	"context"
	"entmodule"
	"fmt"

	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type (
	KnowledgeBiz interface {
		SearchKnowledgeSegment(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error)
		RecallKnowledgeSegment(ctx context.Context, content string, kids []int) ([]*types.KnowledgeSegment, error)
		CreateKnowledgeDocument(ctx context.Context, args *data.UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error)
		BatchCreateKnowledgeDocuments(ctx context.Context, args []*data.UpsertDocumentArgs) ([]*ent.AiKnowledgeDocument, error)
		CreateKnowledge(ctx context.Context, args *data.UpsertKnowledgeArgs) (*ent.AiKnowledge, error)
		UpdateKnowledge(ctx context.Context, args *data.UpsertKnowledgeArgs) (*ent.AiKnowledge, error)
		DeleteKnowledge(ctx context.Context, id int) error
		GetKnowledge(ctx context.Context, id int) (*ent.AiKnowledge, error)
		GetUserMasterKnowledge(ctx context.Context, userID int) (*ent.AiKnowledge, error)
		ListKnowledge(ctx context.Context, args *data.ListKnowledgeArgs) (*data.ListKnowledgeResult, error)
		UpdateKnowledgeDocument(ctx context.Context, args *data.UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error)
		DeleteKnowledgeDocument(ctx context.Context, id int) error
		BatchDeleteKnowledgeDocuments(ctx context.Context, ids []int) error
		GetKnowledgeDocument(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error)
		GetKnowledgeDocuments(ctx context.Context, ids []int) ([]*ent.AiKnowledgeDocument, error)
		ListKnowledgeDocument(ctx context.Context, args *data.ListKnowledgeDocumentArgs) (*data.ListKnowledgeDocumentResult, error)
		UpdateDocumentStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeDocument, error)
		GetKnowledgeSegments(ctx context.Context, ids []int) ([]*types.KnowledgeSegment, error)
		GetKnowledgeSegment(ctx context.Context, id int) (*ent.AiKnowledgeSegment, error)
		ListKnowledgeSegments(ctx context.Context, args *data.ListKnowledgeSegmentArgs) (*data.ListKnowledgeSegmentResult, error)
		Retrieve(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error)
		CopyKnowledgeDocument(ctx context.Context, id int, version string) (*ent.AiKnowledgeDocument, error)
		ChangeKnowledgeDocumentOwner(ctx context.Context, id int, oldKID, newKID int) (*ent.AiKnowledgeDocument, error)
		GetSupportTextParseTypes(ctx context.Context) *types.TextParseSupport
	}

	knowledgeBiz struct {
		kc          data.KnowledgeClient
		kdc         data.KnowledgeDocumentClient
		ksc         data.KnowledgeSegmentClient
		fc          rpc.FileClient
		conf        *conf.Bootstrap
		indexer     indexer.Indexer
		vectorStore vector.VectorStore
		ie          *ingestion.IngestEngine
		re          *retrieval.RetrieveEngine
		embedder    embedding.Embedder

		l *log.Helper
	}
)

func NewKnowledgeBiz(kc data.KnowledgeClient, kdc data.KnowledgeDocumentClient, ksc data.KnowledgeSegmentClient, fc rpc.FileClient,
	l log.Logger, conf *conf.Bootstrap, indexer indexer.Indexer, ie *ingestion.IngestEngine, re *retrieval.RetrieveEngine, embedder embedding.Embedder,
	vectorStore vector.VectorStore) (KnowledgeBiz, error) {
	return &knowledgeBiz{
		kc:          kc,
		kdc:         kdc,
		ksc:         ksc,
		fc:          fc,
		l:           log.NewHelper(l, log.WithMessageKey("biz-knowledge")),
		conf:        conf,
		indexer:     indexer,
		ie:          ie,
		re:          re,
		vectorStore: vectorStore,
		embedder:    embedder,
	}, nil
}

func (b *knowledgeBiz) RecallKnowledgeSegment(ctx context.Context, content string, kids []int) ([]*types.KnowledgeSegment, error) {
	var res []*types.KnowledgeSegment
	for _, kid := range kids {
		segs, err := b.SearchKnowledgeSegment(ctx, &types.SegmentSearchArgs{
			KnowledgeID: kid,
			Content:     content,
		})
		if err != nil {
			return nil, nil
		}
		res = append(res, segs...)
	}
	return res, nil
}

func (b *knowledgeBiz) CreateKnowledgeDocument(ctx context.Context, args *data.UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error) {
	return b.upsertKnowledgeDocument(ctx, args)
}

func (b *knowledgeBiz) upsertKnowledgeDocument(ctx context.Context, args *data.UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error) {
	args.Process = types.DocumentPending
	// 1. 获取文件下载链接
	dUrlResp, err := b.fc.GetFileUrl(ctx, []string{args.Url})
	if err != nil {
		return nil, err
	}
	// 2. 文档信息写入数据库
	doc, err := b.kdc.Upsert(ctx, args)
	info := &ingestion.DocumentInfo{
		KnowledgeID: args.KnowledgeID,
		Name:        args.Name,
		Url:         dUrlResp.Urls[0].Url,
		Version:     args.Version,
	}
	if err != nil {
		return nil, fmt.Errorf("failed to upsert document: %w", err)
	}
	info.ID = doc.ID
	err = b.ie.Ingest(ctx, info, args.Url)
	if err != nil {
		return nil, fmt.Errorf("failed to update document %d status: %v", info.ID, err)
	}
	return doc, nil
}

func (b *knowledgeBiz) BatchCreateKnowledgeDocuments(ctx context.Context, args []*data.UpsertDocumentArgs) ([]*ent.AiKnowledgeDocument, error) {
	uris := lo.Map(args, func(item *data.UpsertDocumentArgs, index int) string {
		return item.Url
	})
	// 1. 获取文件下载链接
	dUrlResp, err := b.fc.GetFileUrl(ctx, uris)
	if err != nil {
		return nil, err
	}
	infos := make([]*ingestion.DocumentInfo, len(args))
	for i, arg := range args {
		arg.Process = types.DocumentPending
		infos[i] = &ingestion.DocumentInfo{
			KnowledgeID: arg.KnowledgeID,
			Name:        arg.Name,
			Url:         dUrlResp.Urls[i].Url,
		}
	}

	docs, err := b.kdc.BatchCreate(ctx, args)
	if err != nil {
		return nil, err
	}
	// 2. 批量建立索引
	err = b.ie.BatchIngest(ctx, infos, uris)
	if err != nil {
		return nil, err
	}

	return docs, nil
}

func (b *knowledgeBiz) SearchKnowledgeSegment(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error) {
	knowledge, err := b.kdc.GetActiveByID(ctx, args.KnowledgeID)
	if err != nil || knowledge == nil {
		return nil, err
	}

	// 1. 检索文档
	// TODO: 根据knowledge配置模型用 retriver router 去检索文档
	chain, err := b.buildRetrieveChain(ctx, nil)
	if err != nil {
		return nil, err
	}
	docs, err := chain.Invoke(ctx, args.Content,
		compose.WithRetrieverOption(retriever.WithTopK(args.TopK*3),
			retriever.WithScoreThreshold(args.Similarity),
			milvus2.WithFilter(fmt.Sprintf("knowledge_id = %d", knowledge.ID))),
	)

	// 2. 更新召回次数
	vectorIDs := make([]string, 0, len(docs))
	for _, doc := range docs {
		vectorIDs = append(vectorIDs, doc.ID)
	}
	segments, err := b.ksc.GetByVectorIDs(ctx, vectorIDs)
	if err != nil {
		return nil, err
	}
	segIDs := make([]int, 0, len(segments))
	for _, segment := range segments {
		segIDs = append(segIDs, segment.ID)
	}
	err = b.ksc.UpdateRetrievalCountByIDs(ctx, segIDs, 1)
	if err != nil {
		return nil, err
	}

	// 3. 按 vectorIDs 顺序排序 segments
	// 3.1 创建 vectorID 到 segment 的映射
	segmentMap := make(map[string]*ent.AiKnowledgeSegment)
	for _, seg := range segments {
		segmentMap[seg.VectorID] = seg
	}
	// 3.2 检查 segments 与 vectorIDs 是否长度一致
	if len(segments) != len(vectorIDs) {
		b.l.Warnf("len(segments) != len(vectorIDs), segments: %v, vectorIDs: %v", segments, vectorIDs)
		segments = make([]*ent.AiKnowledgeSegment, len(vectorIDs))
	}

	// 3.2 按照 vectorIDs 顺序填充 segments
	for i, vectorID := range vectorIDs {
		if seg, ok := segmentMap[vectorID]; ok {
			segments[i] = seg
		} else {
			// 处理不存在的情况，例如设置为 nil 或跳过
			segments[i] = nil
		}
	}

	segsResp := make([]*types.KnowledgeSegment, 0, len(segments))
	for i, seg := range segments {
		if seg == nil {
			continue
		}
		segsResp = append(segsResp, &types.KnowledgeSegment{
			ID:          seg.ID,
			DocumentID:  seg.DocumentID,
			KnowledgeID: knowledge.ID,
			Content:     docs[i].Content,
			ContentLen:  seg.ContentLength,
			Tokens:      seg.Tokens,
			Score:       docs[i].Score(),
			VectorID:    seg.VectorID,
		})
	}
	return segsResp, nil
}

func (b *knowledgeBiz) buildRetrieveChain(ctx context.Context, emb embedding.Embedder) (compose.Runnable[string, []*schema.Document], error) {
	retriever, err := retrieval.NewMilvusRetriever(b.conf.Data.Milvus, emb)
	if err != nil {
		b.l.Errorf("failed to initialize retriever: %v", err)
	}

	reranker, err := rerank.NewScoreReranker(&rerank.ScoreRerankerConfig{
		TopN:           5,
		ScoreThreshold: 0.6,
	})

	chain := compose.NewChain[string, []*schema.Document]()

	chain.AppendRetriever(retriever)
	chain.AppendDocumentTransformer(reranker)
	return chain.Compile(ctx)
}

func (b *knowledgeBiz) Retrieve(ctx context.Context, args *types.SegmentSearchArgs) ([]*types.KnowledgeSegment, error) {
	return b.re.Retrieve(ctx, args)
}

func (b *knowledgeBiz) CreateKnowledge(ctx context.Context, args *data.UpsertKnowledgeArgs) (*ent.AiKnowledge, error) {
	return b.kc.Upsert(ctx, args)
}

func (b *knowledgeBiz) UpdateKnowledge(ctx context.Context, args *data.UpsertKnowledgeArgs) (*ent.AiKnowledge, error) {
	// Check if the knowledge exists
	_, err := b.validateKnowledge(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	return b.kc.Upsert(ctx, args)
}

func (b *knowledgeBiz) DeleteKnowledge(ctx context.Context, id int) error {
	// Check if the knowledge exists
	_, err := b.validateKnowledge(ctx, id)
	if err != nil {
		return err
	}
	return b.kc.Delete(ctx, id)
}

func (b *knowledgeBiz) GetKnowledge(ctx context.Context, id int) (*ent.AiKnowledge, error) {
	k, err := b.kc.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge: %w", err)
	}
	return k, nil
}

func (b *knowledgeBiz) GetUserMasterKnowledge(ctx context.Context, userID int) (*ent.AiKnowledge, error) {
	k, err := b.kc.GetUserMaster(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user master knowledge: %w", err)
	}
	return k, nil
}

func (b *knowledgeBiz) validateKnowledge(ctx context.Context, id int) (*ent.AiKnowledge, error) {
	k, err := b.kc.GetByID(ctx, id)
	if err != nil || k == nil {
		return nil, fmt.Errorf("failed to get knowledge: %w", err)
	}
	return k, nil
}

func (b *knowledgeBiz) validateKnowledges(ctx context.Context, ids []int) ([]*ent.AiKnowledge, error) {
	ks, err := b.kc.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledges: %w", err)
	}
	return ks, nil
}

func (b *knowledgeBiz) ListKnowledge(ctx context.Context, args *data.ListKnowledgeArgs) (*data.ListKnowledgeResult, error) {
	newCtx := context.WithValue(ctx, data.LoadKnowledgeDocument{}, true)
	return b.kc.List(newCtx, args)
}

func (b *knowledgeBiz) UpdateKnowledgeDocument(ctx context.Context, args *data.UpsertDocumentArgs) (*ent.AiKnowledgeDocument, error) {
	old, err := b.validateKnowledgeDocument(ctx, args.ID)
	if err != nil {
		return nil, err
	}
	_, err = b.validateKnowledge(ctx, args.KnowledgeID)
	if err != nil {
		return nil, err
	}
	doc, err := b.kdc.Upsert(ctx, args)
	if err != nil {
		return nil, err
	}

	if doc.Process == types.DocumentSuccess && args.SegmentMaxTokens != old.SegmentMaxTokens {
		// 删除旧的文档切片
		err = b.deleteKnowledgeSegmentsByDocID(ctx, doc.KnowledgeID, doc.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to delete segments by documentID: %w", err)
		}
		// 重新创建文档切片
		_, err = b.upsertKnowledgeDocument(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("failed to rebuild knowledge document: %w", err)
		}
	}
	return doc, nil
}

func (b *knowledgeBiz) validateKnowledgeDocument(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error) {
	// Check if the knowledge document exists
	d, err := b.kdc.GetByID(ctx, id)
	if err != nil || d == nil {
		return nil, fmt.Errorf("failed to get knowledge: %w", err)
	}
	return d, nil
}

func (b *knowledgeBiz) DeleteKnowledgeDocument(ctx context.Context, id int) error {
	// 1. 校验文档是否存在
	doc, err := b.validateKnowledgeDocument(ctx, id)
	if err != nil {
		return err
	}
	// 2. 删除文档
	err = b.kc.Delete(ctx, id)
	if err != nil {
		return err
	}
	// 3. 删除向量库中所有切片
	return b.deleteKnowledgeSegmentsByDocID(ctx, doc.KnowledgeID, doc.ID)
}

func (b *knowledgeBiz) BatchDeleteKnowledgeDocuments(ctx context.Context, ids []int) error {
	_, err := b.kdc.BatchDelete(ctx, ids)
	return err
}

func (b *knowledgeBiz) GetKnowledgeDocument(ctx context.Context, id int) (*ent.AiKnowledgeDocument, error) {
	d, err := b.kdc.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge document: %w", err)
	}
	return d, nil
}

func (b *knowledgeBiz) GetKnowledgeDocuments(ctx context.Context, ids []int) ([]*ent.AiKnowledgeDocument, error) {
	ids = lo.Uniq(ids)
	docs, err := b.kdc.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge documents: %w", err)
	}
	return docs, nil
}

func (b *knowledgeBiz) ListKnowledgeDocument(ctx context.Context, args *data.ListKnowledgeDocumentArgs) (*data.ListKnowledgeDocumentResult, error) {
	newCtx := context.WithValue(ctx, data.LoadKnowledgeDocument{}, true)
	return b.kdc.List(newCtx, args)
}

func (b *knowledgeBiz) UpdateDocumentStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeDocument, error) {
	_, err := b.validateKnowledgeDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.kdc.UpdateStatus(ctx, id, status)
}

func (b *knowledgeBiz) CreateKnowledgeSegment(ctx context.Context, args *types.KnowledgeSegment) (int, error) {
	// 1.1 校验文档是否存在
	d, err := b.validateKnowledgeDocument(ctx, args.DocumentID)
	if err != nil {
		return 0, err
	}
	// 1.2 校验知识库是否存在
	_, err = b.validateKnowledge(ctx, args.KnowledgeID)
	if err != nil {
		return 0, err
	}
	// 1.3 校验 token 数量
	tokens, err := utils.CountTokens(args.Content, "")
	if err != nil {
		return 0, err
	}
	if tokens > d.SegmentMaxTokens {
		return 0, fmt.Errorf("segment max tokens exceeded")
	}

	// 2. 保存段落
	seg, err := b.ksc.Upsert(ctx, args)
	if err != nil {
		return 0, err
	}
	doc := &schema.Document{
		ID:      uuid.New().String(),
		Content: args.Content,
	}

	// 3. 向量化
	_, err = b.indexer.Store(ctx, []*schema.Document{doc})
	if err != nil {
		return 0, err
	}
	return seg.ID, nil
}

func (b *knowledgeBiz) UpdateKnowledgeSegmentStatus(ctx context.Context, id int, status entmodule.Status) (*ent.AiKnowledgeSegment, error) {
	_, err := b.validateKnowledgeSegment(ctx, id)
	if err != nil {
		return nil, err
	}
	return b.ksc.UpdateStatus(ctx, id, status)
}

func (b *knowledgeBiz) validateKnowledgeSegment(ctx context.Context, id int) (*ent.AiKnowledgeSegment, error) {
	// Check if knowledge segment exists
	existed, err := b.ksc.GetByID(ctx, id)
	if err != nil || existed == nil {
		return nil, fmt.Errorf("failed to get knowledge: %w", err)
	}
	return existed, nil
}

func (b *knowledgeBiz) DeleteKnowledgeSegment(ctx context.Context, id int) error {
	_, err := b.validateKnowledgeSegment(ctx, id)
	if err != nil {
		return err
	}
	return b.ksc.Delete(ctx, id)
}

func (b *knowledgeBiz) GetKnowledgeSegment(ctx context.Context, id int) (*ent.AiKnowledgeSegment, error) {
	seg, err := b.ksc.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get knowledge: %w", err)
	}
	return seg, nil
}

func (b *knowledgeBiz) UpdateKnowledgeSegment(ctx context.Context, seg *types.KnowledgeSegment) (int, error) {
	// 1. 校验切片是否存在
	existed, err := b.validateKnowledgeSegment(ctx, seg.ID)
	if err != nil {
		return 0, err
	}

	// 2. 删除向量
	err = b.deleteSegmentVector(ctx, existed)
	if err != nil {
		return 0, err
	}

	// 3.1 更新切片
	newSeg, err := b.ksc.Upsert(ctx, seg)
	if err != nil {
		return 0, fmt.Errorf("failed to update segment: %w", err)
	}
	// 3.2 重新向量化
	if newSeg.Status == entmodule.StatusActive {
		doc := &schema.Document{
			ID:      uuid.New().String(),
			Content: seg.Content,
		}
		_, err = b.indexer.Store(ctx, []*schema.Document{doc})
	}

	return existed.ID, nil
}

func (b *knowledgeBiz) ListKnowledgeSegments(ctx context.Context, args *data.ListKnowledgeSegmentArgs) (*data.ListKnowledgeSegmentResult, error) {
	newCtx := context.WithValue(ctx, data.LoadKnowledgeSegment{}, true)
	newCtx = context.WithValue(newCtx, data.LoadDocumentSegment{}, true)
	return b.ListKnowledgeSegments(newCtx, args)
}

func (b *knowledgeBiz) deleteSegmentVector(ctx context.Context, seg *ent.AiKnowledgeSegment) error {
	if seg.VectorID == "" {
		return nil
	}
	// 1. 更新向量ID
	err := b.ksc.UpdateVectorID(ctx, seg.ID, "")
	if err != nil {
		return fmt.Errorf("failed to update vector id: %w", err)
	}
	// 2. 删除向量
	err = b.vectorStore.Delete(ctx, []string{seg.VectorID})
	if err != nil {
		return fmt.Errorf("failed to delete vector: %w", err)
	}
	return nil
}

func (b *knowledgeBiz) deleteSegmentVectors(ctx context.Context, segs []*ent.AiKnowledgeSegment) error {
	segIDs := make([]int, len(segs))
	vids := make([]string, len(segs))
	for i, seg := range segs {
		segIDs[i] = seg.ID
		vids[i] = seg.VectorID
	}
	// 1. 更新向量ID
	err := b.ksc.SetEmptyVectorID(ctx, segIDs)
	if err != nil {
		return fmt.Errorf("failed to set empty vector id: %w", err)
	}
	// 2. 删除向量
	err = b.vectorStore.Delete(ctx, vids)
	if err != nil {
		return fmt.Errorf("failed to delete vector: %w", err)
	}
	return nil
}

func (b *knowledgeBiz) ReIndexByKnowledgeID(ctx context.Context, knowledgeID int) error {
	// 1 校验知识库是否存在
	_, err := b.validateKnowledge(ctx, knowledgeID)
	if err != nil {
		return err
	}
	// 2.1 查询知识库下所有启用状态的文档
	docs, err := b.kdc.GetActiveByKnowledgeID(ctx, knowledgeID)
	if err != nil {
		return fmt.Errorf("failed to get documents for knowledge %d: %w", knowledgeID, err)
	}
	docIDs := lo.Map(docs, func(doc *ent.AiKnowledgeDocument, _ int) int {
		return doc.ID
	})
	// 2.2 查询知识库下所有启用状态的切片
	segs, err := b.ksc.GetActiveByDocumentIDs(ctx, docIDs)
	if err != nil {
		return fmt.Errorf("failed to get segments for knowledge %d: %w", knowledgeID, err)
	}
	if len(segs) == 0 {
		return nil
	}

	vids := lo.Map(segs, func(seg *ent.AiKnowledgeSegment, _ int) string {
		return seg.VectorID
	})
	contents, err := b.vectorStore.GetContentByIDs(ctx, vids)
	if err != nil {
		return err
	}
	// 3. 遍历所有切片，重新index
	for i, seg := range segs {
		if err := b.deleteSegmentVector(ctx, seg); err != nil {
			return fmt.Errorf("failed to delete segment vector: %w", err)
		}
		// 3.1 重新向量化
		doc := &schema.Document{
			ID:      uuid.New().String(),
			Content: contents[i],
		}
		_, err = b.indexer.Store(ctx, []*schema.Document{doc})
		if err != nil {
			return fmt.Errorf("failed to store document: %w", err)
		}
	}
	b.l.Infof("reindex knowledge %d success: %d segments", knowledgeID, len(segs))
	return nil
}

func (b *knowledgeBiz) UpdateKnowledgeDocumentStatus(ctx context.Context, id int, status entmodule.Status) error {
	// 1. 校验文档是否存在
	doc, err := b.validateKnowledgeDocument(ctx, id)
	if err != nil {
		return err
	}
	// 2. 更新文档状态
	_, err = b.kdc.UpdateStatus(ctx, id, status)
	if err != nil {
		return fmt.Errorf("failed to update document status: %w", err)
	}
	// 3. 处理文档切片
	if status == entmodule.StatusActive {
		// 3.1 获取文件下载URL
		dUrlResp, err := b.fc.GetFileUrl(ctx, []string{doc.URL})
		if err != nil {
			return err
		}
		// 3.2 重新建立索引
		info := &ingestion.DocumentInfo{
			KnowledgeID: id,
			Name:        doc.Name,
			Url:         doc.URL,
			ID:          doc.ID,
		}
		go func() {
			err = b.ie.Ingest(context.Background(), info, dUrlResp.Urls[0].Url)
		}()
	} else if status == entmodule.StatusInactive {
		return b.deleteKnowledgeSegmentsByDocID(ctx, doc.KnowledgeID, doc.ID)
	}
	return nil
}

func (b *knowledgeBiz) deleteKnowledgeSegmentsByDocID(ctx context.Context, kid int, docID int) error {
	// 1. 查询需要删除文档段落
	segs, err := b.ksc.GetActiveByDocumentIDs(ctx, []int{docID})
	if err != nil {
		return fmt.Errorf("failed to get segments for document %d: %w", docID, err)
	}
	if len(segs) == 0 {
		return nil
	}

	// 2. 删除所有切片向量
	segIDs := lo.Map(segs, func(seg *ent.AiKnowledgeSegment, index int) int {
		return seg.ID
	})
	_, err = b.ksc.BatchDelete(ctx, segIDs)
	if err != nil {
		return fmt.Errorf("failed to delete segments for document %d: %w", docID, err)
	}

	// 3. 删除向量库中所有切片
	err = b.vectorStore.DeleteByDocID(ctx, kid, docID)
	if err != nil {
		return fmt.Errorf("failed to delete vectors for document %d: %w", docID, err)
	}
	return nil
}

func (b *knowledgeBiz) GetKnowledgeSegments(ctx context.Context, ids []int) ([]*types.KnowledgeSegment, error) {
	segs, err := b.ksc.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get segments for knowledge ids %v: %w", ids, err)
	}
	if len(segs) != len(ids) {
		b.l.Warnf("get segments for ids %v failed, expect %d segments, but get %d", ids, len(ids), len(segs))
	}

	vids := lo.Map(segs, func(seg *ent.AiKnowledgeSegment, _ int) string {
		return seg.VectorID
	})
	contents, err := b.vectorStore.GetContentByIDs(ctx, vids)
	if err != nil {
		return nil, fmt.Errorf("failed to query segments from vector store: %w", err)
	}
	if len(contents) != len(ids) {
		b.l.Warnf("query segments content for ids %v failed, expect %d contents, but get %d", ids, len(ids), len(contents))
	}
	finalSegs := make([]*types.KnowledgeSegment, len(ids))
	for i, seg := range segs {
		finalSegs[i] = &types.KnowledgeSegment{
			ID:          seg.ID,
			DocumentID:  seg.DocumentID,
			KnowledgeID: seg.KnowledgeID,
			Content:     contents[i],
			ContentLen:  seg.ContentLength,
			Tokens:      seg.Tokens,
			VectorID:    seg.VectorID,
			Status:      seg.Status,
		}
	}
	return finalSegs, nil
}

func (b *knowledgeBiz) CopyKnowledgeDocument(ctx context.Context, id int, version string) (*ent.AiKnowledgeDocument, error) {
	// 1. 校验文档是否存在
	existed, err := b.validateKnowledgeDocument(ctx, id)
	if err != nil {
		return nil, err
	}
	if existed.Version != version {
		return nil, fmt.Errorf("version not match, expect %s, but %s", existed.Version, version)
	}
	// 2. 复制文档
	newDoc, err := b.kdc.Upsert(ctx, &data.UpsertDocumentArgs{
		KnowledgeID:      existed.KnowledgeID,
		Name:             existed.Name,
		Url:              existed.URL,
		Version:          version,
		ContentLen:       existed.ContentLength,
		SegmentMaxTokens: existed.SegmentMaxTokens,
		RetrievalCount:   existed.RetrievalCount,
		Process:          existed.Process,
		Status:           entmodule.StatusActive,
	})
	if err != nil {
		return nil, err
	}
	return newDoc, nil
}

func (b *knowledgeBiz) ChangeKnowledgeDocumentOwner(ctx context.Context, id int, oldKID, newKID int) (*ent.AiKnowledgeDocument, error) {
	u := trans.FromContext(ctx)
	// 1. 校验文档是否存在
	_, err := b.validateKnowledgeDocument(ctx, id)
	if err != nil {
		return nil, err
	}

	// 2.1 校验旧知识库是否存在
	old, err := b.validateKnowledge(ctx, oldKID)
	if err != nil {
		return nil, fmt.Errorf("old knowledge %d not found: %w", oldKID, err)
	}
	permissions := boolset.BooleanSet(u.Group.Permissions)
	if old.UserID != int(u.Id) || !permissions.Enabled(int(constants.GroupPermissionIsAdmin)) {
		return nil, fmt.Errorf("user %d has no permission to change knowledge %d owner", u.Id, oldKID)
	}
	// 2.2 校验新知识库是否存在
	_, err = b.validateKnowledge(ctx, newKID)
	if err != nil {
		return nil, fmt.Errorf("new knowledge %d not found: %w", newKID, err)
	}
	// 2. 更新文档所在知识库
	doc, err := b.kdc.UpdateKnowledgeID(ctx, id, newKID)
	if err != nil {
		return nil, fmt.Errorf("failed to update document owner: %w", err)
	}
	return doc, nil
}

func (b *knowledgeBiz) GetSupportTextParseTypes(_ context.Context) *types.TextParseSupport {
	return &types.TextParseSupport{
		Types: []types.TextParseType{
			types.TextTxt,
			types.TextMarkdown,
			types.TextPDF,
			types.TextHTML,
			types.TextJSON,
			types.TextWord,
			types.TextExcel,
			types.TextCSV,
		},
		MaxFileSize: types.MaxFileSize,
	}
}
