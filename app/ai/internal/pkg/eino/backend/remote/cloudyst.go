package remote

import (
	"ai/internal/data/rpc"
	pbfile "api/api/file/files/v1"
	"bufio"
	"bytes"
	"common/constants"
	"common/request"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

// 1. 定义一个初始缓冲区（比如 64KB）
const initialCapacity = 64 * 1024

// 2. 定义允许的最大行长度（比如 2MB）
const maxCapacity = 2 * 1024 * 1024

const defaultRootURI = string(constants.CloudScheme + "://" + constants.FileSystemMy)

type Config struct {
	Client request.Client
}
type Remote struct {
	fc   rpc.FileClient
	l    *log.Helper
	conf *Config
}

func (r *Remote) LsInfo(ctx context.Context, req *filesystem.LsInfoRequest) ([]filesystem.FileInfo, error) {
	page := 0
	files := make([]filesystem.FileInfo, 0)
	isEnd := false
	for !isEnd {
		resp, err := r.fc.ListDirectory(ctx, req.Path, page)
		if err != nil {
			return nil, err
		}
		for _, entry := range resp.Files {
			files = append(files, filesystem.FileInfo{
				Path:       entry.Path,
				IsDir:      entry.Type == 1,
				Size:       entry.Size,
				ModifiedAt: entry.UpdatedAt.AsTime().Format(time.RFC3339),
			})
		}
		isEnd = resp.Pagination.NextPageToken == ""
		page++
	}
	return files, nil
}

func (r *Remote) Read(ctx context.Context, req *filesystem.ReadRequest) (*filesystem.FileContent, error) {
	// 1. 获取文件url
	urlResp, err := r.fc.GetFileUrl(ctx, []string{req.FilePath})
	if err != nil {
		return nil, err
	}
	url := urlResp.Urls[0].Url

	// 2. 读取文件内容
	content, err := r.read(ctx, url, req.Offset, req.Limit)
	if err != nil {
		return nil, err
	}
	return &filesystem.FileContent{
		Content: content,
	}, nil
}

// read 读取文件内容，默认读取2000行
func (r *Remote) read(ctx context.Context, url string, offset, limit int) (string, error) {
	// 1. 下载文件内容
	// TODO: 性能优化：使用 HTTP Range Header 按字节范围下载
	fileResp := r.conf.Client.Request(http.MethodGet, url, nil, request.WithContext(ctx)).
		CheckHTTPResponse(http.StatusOK)
	if fileResp.Err != nil {
		return "", fileResp.Err
	}
	defer fileResp.Response.Body.Close()
	// 2. 配置带保护的 Scanner，防止 OOM
	scanner := bufio.NewScanner(fileResp.Response.Body)
	buf := make([]byte, initialCapacity)
	scanner.Buffer(buf, maxCapacity)
	// 3. 解析文件内容
	if offset <= 0 {
		offset = 1
	}
	if limit <= 0 {
		limit = 2000
	}
	var result strings.Builder
	lineNum := 1
	linesRead := 0
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if lineNum >= offset {
			// 使用 Bytes() 减少内存拷贝，并手动补回换行符
			result.Write(scanner.Bytes())
			result.WriteByte('\n')
			linesRead++
			if linesRead >= limit {
				break
			}
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("scanner error: %w", err)
	}
	return result.String(), nil
}

func (r *Remote) GrepRaw(ctx context.Context, req *filesystem.GrepRequest) ([]filesystem.GrepMatch, error) {
	if req.Pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	// 1. 文件名匹配参数
	uri := req.Path
	if uri == "" {
		uri = defaultRootURI
	}
	if req.CaseInsensitive {
		uri += "&case_folding=true"
	}
	if req.FileType != "" {
		uri += fmt.Sprintf("&name=*.%s", req.FileType)
	}
	//if req.Glob != "" {
	//	uri += fmt.Sprintf("&name=%s", req.Glob)
	//}
	uri += "&type=file"
	// 2. 获取文件
	files, err := r.fc.ListDirectory(ctx, uri, 0)
	if err != nil {
		return nil, err
	}
	uris := lo.Map(files.Files, func(file *pbfile.FileResponse, index int) string {
		return file.Path
	})
	// 3. 获取文件url
	urlResp, err := r.fc.GetFileUrl(ctx, uris)
	if err != nil {
		return nil, err
	}
	// 3. 文件内容匹配参数
	resultCh := make(chan grepResult, len(urlResp.Urls))

	// 编译正则（支持大小写不敏感）
	regexFlag := ""
	if req.CaseInsensitive {
		regexFlag = "(?i)"
	}
	pattern, err := regexp.Compile(regexFlag + req.Pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var eg errgroup.Group
	for _, urlInfo := range urlResp.Urls {
		eg.Go(func() error {
			// 下载文件内容
			fileResp := r.conf.Client.Request(http.MethodGet, urlInfo.Url, nil, request.WithContext(ctx)).
				CheckHTTPResponse(http.StatusOK)
			if fileResp.Err != nil {
				resultCh <- grepResult{uri: urlInfo.Url, err: err}
				return fileResp.Err
			}
			defer fileResp.Response.Body.Close()

			// 逐行匹配
			var matches []filesystem.GrepMatch
			scanner := bufio.NewScanner(fileResp.Response.Body)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				if pattern.MatchString(line) {
					matches = append(matches, filesystem.GrepMatch{
						Line:    lineNum,
						Content: line,
						Path:    urlInfo.Url,
					})
				}
			}
			if err := scanner.Err(); err != nil {
				resultCh <- grepResult{uri: urlInfo.Url, err: err}
				return err
			}

			resultCh <- grepResult{uri: urlInfo.Url, matches: matches}
			return nil
		})
	}

	// 等待所有goroutine完成后关闭channel
	go func() {
		if err := eg.Wait(); err != nil {
			r.l.Errorf("eg.Wait(): %v", err)
		}
		close(resultCh)
	}()

	// 5. 收集结果
	var grepErrors []error
	var results []filesystem.GrepMatch
	for res := range resultCh {
		if res.err != nil {
			grepErrors = append(grepErrors, fmt.Errorf("file %s: %w", res.uri, res.err))
			continue
		}
		if len(res.matches) > 0 {
			results = append(results, res.matches...)
		}
	}

	// 部分失败时记录日志但不中断
	if len(grepErrors) > 0 {
		r.l.Warnf("grep errors: %v", grepErrors)
	}
	return results, nil
}

func (r *Remote) GlobInfo(ctx context.Context, req *filesystem.GlobInfoRequest) ([]filesystem.FileInfo, error) {
	if req.Path == "" {
		req.Path = defaultRootURI
	}
	if req.Pattern != "" {
		req.Path += fmt.Sprintf("?name=%s", req.Pattern)
	}
	req.Path += "&type=file"

	// 获取路径下文件信息
	files, err := r.fc.ListDirectory(ctx, req.Path, 0)
	if err != nil {
		return nil, err
	}
	infos := make([]filesystem.FileInfo, 0)
	for _, entry := range files.Files {
		infos = append(infos, filesystem.FileInfo{
			Path:       entry.Path,
			IsDir:      entry.Type == 1,
			Size:       entry.Size,
			ModifiedAt: entry.UpdatedAt.AsTime().Format(time.RFC3339),
		})
	}
	return infos, nil
}

func (r *Remote) Write(ctx context.Context, req *filesystem.WriteRequest) error {
	if req.FilePath == "" {
		return fmt.Errorf("file path is required")
	}
	fileResponse, err := r.fc.GetFileInfo(ctx, req.FilePath)
	prev := ""
	if err != nil {
		if errors.IsNotFound(err) {
			r.l.Debugf("file %s not found, create it", req.FilePath)
		} else {
			return err
		}
	} else {
		// 文件存在，prev 为文件的主实体版本id
		prev = fileResponse.PrimaryEntity
	}

	_, err = r.fc.PutContentString(ctx, req.FilePath, prev, req.Content)
	return err
}

func (r *Remote) Edit(ctx context.Context, req *filesystem.EditRequest) error {
	if req.FilePath == "" {
		return fmt.Errorf("file path is required")
	}
	if req.OldString == "" {
		return fmt.Errorf("old string is required")
	}
	if req.OldString == req.NewString {
		return fmt.Errorf("new string must be different from old string")
	}
	// 1. 读取文件内容
	text, err := r.Read(ctx, &filesystem.ReadRequest{FilePath: req.FilePath})
	if err != nil {
		return err
	}

	count := strings.Count(text.Content, req.OldString)

	if count == 0 {
		return fmt.Errorf("string not found in file: '%s'", req.OldString)
	}
	if count > 1 && !req.ReplaceAll {
		return fmt.Errorf("string '%s' appears multiple times. Use replace_all=True to replace all occurrences", req.OldString)
	}

	// 2. 替换字符串
	var newText string
	if req.ReplaceAll {
		newText = strings.Replace(text.Content, req.OldString, req.NewString, -1)
	} else {
		newText = strings.Replace(text.Content, req.OldString, req.NewString, 1)
	}
	// 3. 写入新内容
	return r.Write(ctx, &filesystem.WriteRequest{
		FilePath: req.FilePath,
		Content:  newText,
	})
}

// streamEdit 边读、边匹配、边替换、边写
func (r *Remote) streamEdit(ctx context.Context, req *filesystem.EditRequest) error {
	if req.FilePath == "" {
		return fmt.Errorf("file path is required")
	}
	if req.OldString == "" {
		return fmt.Errorf("old string is required")
	}
	if req.OldString == req.NewString {
		return fmt.Errorf("new string must be different from old string")
	}
	// 1. 获取文件下载url
	urlResp, err := r.fc.GetFileUrl(ctx, []string{req.FilePath})
	if err != nil {
		return err
	}
	url := urlResp.Urls[0].Url

	// 2. 流式读取文件内容
	fileResp := r.conf.Client.Request(http.MethodGet, url, nil, request.WithContext(ctx)).
		CheckHTTPResponse(http.StatusOK)
	if fileResp.Err != nil {
		return fileResp.Err
	}
	rc := fileResp.Response.Body
	defer rc.Close()

	old := []byte(req.OldString)
	neo := []byte(req.NewString)
	overlapLen := len(old) - 1 // 跨块overlap长度

	// io.Pipe 连接替换逻辑与PutContent
	pr, pw := io.Pipe()

	// PutContent 在独立goroutine中消费pr
	uploadErrCh := make(chan error, 1)
	go func() {
		_, err := r.fc.PutContent(ctx, req.FilePath, "", pr, -1)
		uploadErrCh <- err
	}()

	replaceErr := func() error {
		const chunkSize = 32 * 1024
		buf := make([]byte, max(chunkSize, len(old)*2))
		var overlap []byte
		count := 0

		// 3. 替换逻辑写入pw，任何错误通过 pw.CloseWithError 传递给PutContent
		for {
			n, readErr := rc.Read(buf)
			if n == 0 && readErr != nil {
				if readErr == io.EOF {
					break
				}
				return readErr
			}

			// 3.1 当前窗口 = 上次的overlap + 本次chunk
			window := append(overlap, buf[:n]...)
			var processed []byte
			remaining := window

			// 3.2 在窗口内执行替换
			for {
				idx := bytes.Index(remaining, old)
				if idx == -1 {
					break
				}
				count++
				processed = append(processed, remaining[:idx]...)
				processed = append(processed, neo...)
				remaining = remaining[idx+len(old):]
				if !req.ReplaceAll {
					// 非全量替换：后续内容不再扫描，直接透传
					processed = append(processed, remaining...)
					remaining = nil
					break
				}
			}

			// 3.2 确定本次安全写出的部分
			//    末尾 overlapLen 字节留作下次 overlap（除非已到EOF）
			isLast := readErr == io.EOF
			if isLast {
				processed = append(processed, remaining...)
				if _, err := pw.Write(processed); err != nil {
					return err
				}
				break
			}

			// 非末尾：remaining 末尾保留 overlap
			safeEnd := len(remaining)
			if safeEnd > overlapLen {
				safeEnd = len(remaining) - overlapLen
				overlap = append([]byte(nil), remaining[safeEnd:]...)
				processed = append(processed, remaining[:safeEnd]...)
			} else {
				// remaining 本身比 overlapLen 短，全部留作 overlap
				overlap = append([]byte(nil), remaining...)
			}

			if len(processed) > 0 {
				if _, err := pw.Write(processed); err != nil {
					return err
				}
			}
		}

		// 4. ReplaceAll=false 时校验次数
		if !req.ReplaceAll && count != 1 {
			return fmt.Errorf("OldString matched %d times, expected exactly 1", count)
		}
		return nil
	}()

	pw.CloseWithError(replaceErr) // 无论成功失败都关闭pw，通知PutContent结束

	uploadErr := <-uploadErrCh
	if replaceErr != nil {
		return replaceErr
	}
	return uploadErr
}

type grepResult struct {
	uri     string
	matches []filesystem.GrepMatch
	err     error
}
