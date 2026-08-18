package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	es8 "github.com/elastic/go-elasticsearch/v8"

	"IM_Chat_System/internal/model"
)

type Indexer struct {
	client *es8.Client
	index  string
}

func New(url, index string) (*Indexer, error) {
	client, err := es8.NewClient(es8.Config{
		Addresses: []string{url},
	})
	if err != nil {
		return nil, err
	}

	i := &Indexer{client: client, index: index}
	if err := i.ensureIndex(); err != nil {
		return nil, err
	}
	return i, nil
}

// 在 Elasticsearch 服务启动时自动检查并初始化消息索引, 保证索引结构和搜索字段可用
func (i *Indexer) ensureIndex() error {
	res, err := i.client.Indices.Exists([]string{i.index})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == 200 {
		return i.ensureRawSearchFields()
	}

	body := `{
	  "mappings": {
	    "properties": {
	      "id": {"type": "long"},
	      "from_user_id": {"type": "long"},
	      "to_user_id": {"type": "long"},
	      "group_id": {"type": "long"},
	      "content_type": {"type": "keyword"},
	      "content": {"type": "text"},
	      "content_raw": {"type": "keyword", "ignore_above": 32766},
	      "object_url": {"type": "keyword"},
	      "file_name": {"type": "text"},
	      "file_name_raw": {"type": "keyword", "ignore_above": 32766},
	      "created_at": {"type": "date"}
	    }
	  }
	}`
	createRes, err := i.client.Indices.Create(i.index, i.client.Indices.Create.WithBody(strings.NewReader(body)))
	if err != nil {
		return err
	}
	defer createRes.Body.Close()
	if createRes.IsError() {
		return fmt.Errorf("create index failed: %s", createRes.String())
	}
	return i.ensureRawSearchFields()
}

// ensureRawSearchFields 为文本搜索补充保留原始字符的 keyword 字段。
// 标准 text 分词会忽略 ^、括号等表情符号；该字段用于精确的子串匹配。
func (i *Indexer) ensureRawSearchFields() error {
	mapping := `{
	  "properties": {
	    "group_id": {"type": "long"},
	    "content_raw": {"type": "keyword", "ignore_above": 32766},
	    "file_name_raw": {"type": "keyword", "ignore_above": 32766}
	  }
	}`
	res, err := i.client.Indices.PutMapping(
		[]string{i.index},
		strings.NewReader(mapping),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("update search mapping failed: %s", res.String())
	}

	// 旧索引文档在新增字段前没有 keyword 值。只回填缺失字段的文档，
	// 使已有的颜文字也能立即被 ^、*、() 等字符检索到。
	backfill := `{
	  "script": {
	    "lang": "painless",
	    "source": "if (ctx._source.content_raw == null) { ctx._source.content_raw = ctx._source.content; } if (ctx._source.file_name_raw == null) { ctx._source.file_name_raw = ctx._source.file_name; }"
	  },
	  "query": {
	    "bool": {
	      "should": [
	        {"bool": {"must_not": {"exists": {"field": "content_raw"}}}},
	        {"bool": {"must_not": {"exists": {"field": "file_name_raw"}}}}
	      ],
	      "minimum_should_match": 1
	    }
	  }
	}`
	backfillRes, err := i.client.UpdateByQuery(
		[]string{i.index},
		i.client.UpdateByQuery.WithBody(strings.NewReader(backfill)),
		i.client.UpdateByQuery.WithConflicts("proceed"),
	)
	if err != nil {
		return err
	}
	defer backfillRes.Body.Close()
	if backfillRes.IsError() {
		return fmt.Errorf("backfill raw search fields failed: %s", backfillRes.String())
	}
	return nil
}

// 把一条聊天消息 (message) 保存到 Elasticsearch 里面, 建立搜索索引
func (i *Indexer) IndexMessage(ctx context.Context, message model.Message) error {
	body, err := json.Marshal(map[string]any{
		"id":            message.ID,
		"from_user_id":  message.FromUserID,
		"to_user_id":    message.ToUserID,
		"group_id":      message.GroupID,
		"content_type":  message.ContentType,
		"content":       message.Content,
		"content_raw":   message.Content,
		"object_key":    message.ObjectKey,
		"object_url":    message.ObjectURL,
		"file_name":     message.FileName,
		"file_name_raw": message.FileName,
		"file_size":     message.FileSize,
		"created_at":    message.CreatedAt,
	})
	if err != nil {
		return err
	}

	res, err := i.client.Index(
		i.index,
		bytes.NewReader(body),
		i.client.Index.WithContext(ctx),
		i.client.Index.WithDocumentID(strconv.FormatInt(message.ID, 10)),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("index message failed: %s", res.String())
	}
	return nil
}

// 从 Elasticsearch 搜索消息
func (i *Indexer) SearchMessages(ctx context.Context, userID int64, query string, peerID, groupID int64, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 20
	}
	must := []map[string]any{}
	query = strings.TrimSpace(query)
	if query != "" {
		wildcardQuery := "*" + escapeWildcardQuery(query) + "*"
		must = append(must, map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{
						"multi_match": map[string]any{
							"query":  query,
							"fields": []string{"content", "file_name"},
						},
					},
					{
						"multi_match": map[string]any{
							"query":  query,
							"type":   "phrase_prefix",
							"fields": []string{"content", "file_name"},
						},
					},
					{
						"wildcard": map[string]any{
							"content_raw": map[string]any{
								"value":            wildcardQuery,
								"case_insensitive": true,
							},
						},
					},
					{
						"wildcard": map[string]any{
							"file_name_raw": map[string]any{
								"value":            wildcardQuery,
								"case_insensitive": true,
							},
						},
					},
				},
				"minimum_should_match": 1,
			},
		})
	}

	filter := []map[string]any{
		{
			"bool": map[string]any{
				"should": []map[string]any{
					{"term": map[string]any{"from_user_id": userID}},
					{"term": map[string]any{"to_user_id": userID}},
				},
				"minimum_should_match": 1,
			},
		},
	}
	if groupID > 0 {
		filter = []map[string]any{{"term": map[string]any{"group_id": groupID}}}
	}

	if peerID > 0 {
		filter = append(filter, map[string]any{
			"bool": map[string]any{
				"should": []map[string]any{
					{
						"bool": map[string]any{
							"must": []map[string]any{
								{"term": map[string]any{"from_user_id": userID}},
								{"term": map[string]any{"to_user_id": peerID}},
							},
						},
					},
					{
						"bool": map[string]any{
							"must": []map[string]any{
								{"term": map[string]any{"from_user_id": peerID}},
								{"term": map[string]any{"to_user_id": userID}},
							},
						},
					},
				},
				"minimum_should_match": 1,
			},
		})
	}

	body, err := json.Marshal(map[string]any{
		"size": limit,
		"sort": []map[string]any{{"created_at": map[string]any{"order": "desc"}}},
		"query": map[string]any{
			"bool": map[string]any{
				"must":   must,
				"filter": filter,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	res, err := i.client.Search(
		i.client.Search.WithContext(ctx),
		i.client.Search.WithIndex(i.index),
		i.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("search failed: %s", res.String())
	}

	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				Source struct {
					ID          int64  `json:"id"`
					FromUserID  int64  `json:"from_user_id"`
					ToUserID    int64  `json:"to_user_id"`
					GroupID     *int64 `json:"group_id"`
					ContentType string `json:"content_type"`
					Content     string `json:"content"`
					ObjectKey   string `json:"object_key"`
					ObjectURL   string `json:"object_url"`
					FileName    string `json:"file_name"`
					FileSize    int64  `json:"file_size"`
					CreatedAt   string `json:"created_at"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	messages := make([]model.Message, 0, len(parsed.Hits.Hits))
	for _, hit := range parsed.Hits.Hits {
		createdAt, _ := time.Parse(time.RFC3339, hit.Source.CreatedAt)
		messages = append(messages, model.Message{
			ID:          hit.Source.ID,
			FromUserID:  hit.Source.FromUserID,
			ToUserID:    hit.Source.ToUserID,
			GroupID:     hit.Source.GroupID,
			ContentType: hit.Source.ContentType,
			Content:     hit.Source.Content,
			ObjectKey:   hit.Source.ObjectKey,
			ObjectURL:   hit.Source.ObjectURL,
			FileName:    hit.Source.FileName,
			FileSize:    hit.Source.FileSize,
			CreatedAt:   createdAt,
		})
	}
	return messages, nil
}

func (i *Indexer) Close() error { return nil }

func escapeWildcardQuery(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"?", "\\?",
	)
	return replacer.Replace(value)
}
