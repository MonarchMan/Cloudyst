package retrieval

import (
	"ai/internal/conf"
	"ai/internal/data"
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/retriever/milvus2"
	"github.com/cloudwego/eino-ext/components/retriever/milvus2/search_mode"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

type (
	RetrieverNode struct {
		kdc  data.KnowledgeDocumentClient
		ksc  data.KnowledgeSegmentClient
		l    *log.Helper
		conf *RetrieverConfig
	}

	RetrieverConfig struct {
		Retriever retriever.Retriever
	}
)

func (r *RetrieverNode) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	documents, err := r.conf.Retriever.Retrieve(ctx, query, opts...)
	if err != nil {
		return nil, err
	}
	vectorIDs := make([]string, 0, len(documents))
	for _, doc := range documents {
		vectorIDs = append(vectorIDs, doc.ID)
	}
	segments, err := r.ksc.GetByVectorIDs(ctx, vectorIDs)
	if err != nil {
		return nil, err
	}
	segIDs := make([]int, 0, len(segments))
	for _, segment := range segments {
		segIDs = append(segIDs, segment.ID)
	}
	err = r.ksc.UpdateRetrievalCountByIDs(ctx, segIDs, 1)
	if err != nil {
		return nil, err
	}
	return documents, nil
}

type SearchMode string

const (
	Approximate SearchMode = "approximate"
	Hybrid      SearchMode = "hybrid"
	Iterator    SearchMode = "Iterator"
	Scalar      SearchMode = "Scalar"
	Range       SearchMode = "Range"
	Sparse      SearchMode = "Sparse"
)

func NewMilvusRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	sm := SearchMode(conf.SearchMode)
	if sm == "" {
		sm = Hybrid
	}
	switch sm {
	case Approximate:
		return NewMilvusApproximateRetriever(conf, emb)
	case Hybrid:
		return NewMilvusHybridRetriever(conf, emb)
	case Iterator:
		return NewMilvusIteratorRetriever(conf, emb)
	case Scalar:
		return NewMilvusScalarRetriever(conf, emb)
	case Range:
		return NewMilvusRangeRetriever(conf, emb)
	case Sparse:
		return NewMilvusSparseRetriever(conf, emb)
	default:
		return nil, fmt.Errorf("invalid search mode: %s", conf.SearchMode) // 无效的搜索模式
	}
}

func NewMilvusApproximateRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	approximateMode := search_mode.NewApproximate(milvus2.COSINE)
	// 创建 retriever
	return NewMilvusRetrieverWithMode(approximateMode, conf, emb)
}

func NewMilvusHybridRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	dmType := milvus2.MetricType(conf.MetricType.Dense)
	if dmType == "" {
		dmType = milvus2.COSINE
	}
	smType := milvus2.MetricType(conf.MetricType.Sparse)
	if smType == "" {
		smType = milvus2.BM25
	}
	hybridMode := search_mode.NewHybrid(
		milvusclient.NewRRFReranker().WithK(60), // RRF 重排序器
		&search_mode.SubRequest{
			VectorField: "vector",            // 稠密向量字段
			VectorType:  milvus2.DenseVector, // 默认值，可省略
			TopK:        10,
			MetricType:  dmType,
		},
		// 稀疏子请求 (Sparse SubRequest)
		&search_mode.SubRequest{
			VectorField: "sparse_vector",      // 稀疏向量字段
			VectorType:  milvus2.SparseVector, // 指定稀疏类型
			TopK:        10,
			MetricType:  smType, // 使用 BM25 或 IP
		},
	)
	// 创建 retriever
	return NewMilvusRetrieverWithMode(hybridMode, conf, emb)
}

func NewMilvusIteratorRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	dmType := milvus2.MetricType(conf.MetricType.Dense)
	if dmType == "" {
		dmType = milvus2.COSINE
	}
	iteratorMode := search_mode.NewIterator(dmType, 100).
		WithSearchParams(map[string]string{"ef": "100"})
	// 创建 retriever
	return NewMilvusRetrieverWithMode(iteratorMode, conf, emb)
}

func NewMilvusScalarRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	scalarMode := search_mode.NewScalar()
	return NewMilvusRetrieverWithMode(scalarMode, conf, emb)
}

func NewMilvusRangeRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	dmType := milvus2.MetricType(conf.MetricType.Dense)
	if dmType == "" {
		dmType = milvus2.COSINE
	}
	rangeMode := search_mode.NewRange(dmType, 0.5).
		WithRangeFilter(0.1)
	return NewMilvusRetrieverWithMode(rangeMode, conf, emb)
}

func NewMilvusSparseRetriever(conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	smType := milvus2.MetricType(conf.MetricType.Sparse)
	if smType == "" {
		smType = milvus2.BM25
	}
	sparseMode := search_mode.NewSparse(smType)
	return NewMilvusRetrieverWithMode(sparseMode, conf, emb)
}

func NewMilvusRetrieverWithMode(mode milvus2.SearchMode, conf *conf.Milvus, emb embedding.Embedder) (retriever.Retriever, error) {
	return milvus2.NewRetriever(context.Background(), &milvus2.RetrieverConfig{
		ClientConfig: &milvusclient.ClientConfig{
			Address:  conf.Addr,
			Username: conf.Username,
			Password: conf.Password,
		},
		Collection: conf.Collection,
		TopK:       int(conf.TopK),
		SearchMode: mode,
		Embedding:  emb,
	})
}
