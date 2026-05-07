package graphrag

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const (
	MetaSourceQuery = "raggraph.source_query"
	MetaSourceRank  = "raggraph.source_rank"
	MetaSourceScore = "raggraph.source_score"
	MetaFusionScore = "raggraph.fusion_score"
	MetaHitCount    = "raggraph.hit_count"
)

type RetrievalResult struct {
	Query     string
	Documents []*schema.Document
}

type FusionConfig struct {
	RRFK   float64
	TopN   int
	Dedupe bool
}

func FuseRetrievalResults(results []RetrievalResult, conf *FusionConfig) []*schema.Document {
	if conf == nil {
		conf = &FusionConfig{}
	}
	rrfK := conf.RRFK
	if rrfK <= 0 {
		rrfK = 60
	}
	dedupe := true
	if conf != nil {
		dedupe = conf.Dedupe
	}

	if !dedupe {
		return flattenResults(results, conf.TopN)
	}

	type fused struct {
		doc         *schema.Document
		score       float64
		bestRank    int
		bestQuery   string
		sourceScore float64
		hitCount    int
	}

	items := map[string]*fused{}
	for _, result := range results {
		for rank, doc := range result.Documents {
			if doc == nil {
				continue
			}
			key := documentKey(doc)
			score := 1 / (rrfK + float64(rank+1))
			if existing, ok := items[key]; ok {
				existing.score += score
				existing.hitCount++
				if rank+1 < existing.bestRank {
					existing.bestRank = rank + 1
					existing.bestQuery = result.Query
					existing.sourceScore = doc.Score()
					existing.doc = cloneDocument(doc)
				}
				continue
			}
			items[key] = &fused{
				doc:         cloneDocument(doc),
				score:       score,
				bestRank:    rank + 1,
				bestQuery:   result.Query,
				sourceScore: doc.Score(),
				hitCount:    1,
			}
		}
	}

	ordered := make([]*fused, 0, len(items))
	for _, item := range items {
		ordered = append(ordered, item)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].score == ordered[j].score {
			return ordered[i].sourceScore > ordered[j].sourceScore
		}
		return ordered[i].score > ordered[j].score
	})

	if conf.TopN > 0 && len(ordered) > conf.TopN {
		ordered = ordered[:conf.TopN]
	}
	docs := make([]*schema.Document, 0, len(ordered))
	for _, item := range ordered {
		if item.doc.MetaData == nil {
			item.doc.MetaData = map[string]any{}
		}
		item.doc.MetaData[MetaSourceQuery] = item.bestQuery
		item.doc.MetaData[MetaSourceRank] = item.bestRank
		item.doc.MetaData[MetaSourceScore] = item.sourceScore
		item.doc.MetaData[MetaFusionScore] = item.score
		item.doc.MetaData[MetaHitCount] = item.hitCount
		item.doc.WithScore(item.score)
		docs = append(docs, item.doc)
	}
	return docs
}

func flattenResults(results []RetrievalResult, topN int) []*schema.Document {
	docs := make([]*schema.Document, 0)
	for _, result := range results {
		for rank, doc := range result.Documents {
			next := cloneDocument(doc)
			if next == nil {
				continue
			}
			if next.MetaData == nil {
				next.MetaData = map[string]any{}
			}
			next.MetaData[MetaSourceQuery] = result.Query
			next.MetaData[MetaSourceRank] = rank + 1
			docs = append(docs, next)
			if topN > 0 && len(docs) >= topN {
				return docs
			}
		}
	}
	return docs
}

func documentKey(doc *schema.Document) string {
	if doc.ID != "" {
		return "id:" + doc.ID
	}
	content := strings.TrimSpace(doc.Content)
	sum := sha256.Sum256([]byte(content))
	return "content:" + hex.EncodeToString(sum[:])
}

func cloneDocument(doc *schema.Document) *schema.Document {
	if doc == nil {
		return nil
	}
	next := &schema.Document{
		ID:       doc.ID,
		Content:  doc.Content,
		MetaData: cloneMeta(doc.MetaData),
	}
	if score := doc.Score(); score != 0 {
		next.WithScore(score)
	}
	if dense := doc.DenseVector(); len(dense) > 0 {
		next.WithDenseVector(dense)
	}
	if sparse := doc.SparseVector(); len(sparse) > 0 {
		next.WithSparseVector(sparse)
	}
	if subIndexes := doc.SubIndexes(); len(subIndexes) > 0 {
		next.WithSubIndexes(subIndexes)
	}
	if extra := doc.ExtraInfo(); extra != "" {
		next.WithExtraInfo(extra)
	}
	return next
}
