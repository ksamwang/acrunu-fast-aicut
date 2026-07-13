package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/modelgateway"
)

const (
	preprocessASRMaxAudioBytes   = 64 << 20
	preprocessASRMaxRequestBytes = preprocessASRMaxAudioBytes + (1 << 20)
	preprocessASRMemoryBytes     = 8 << 20
	preprocessASRTimeBase        = "selection_relative_ms"
)

func (s *Server) handlePreprocessASRTranscribe(c *gin.Context) {
	startedAt := time.Now()
	if c.Request.ContentLength > preprocessASRMaxRequestBytes {
		Fail(c, http.StatusRequestEntityTooLarge, "asr_audio_too_large", "音频文件不能超过 64 MiB")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, preprocessASRMaxRequestBytes)
	parseErr := c.Request.ParseMultipartForm(preprocessASRMemoryBytes)
	if c.Request.MultipartForm != nil {
		defer func() {
			if err := c.Request.MultipartForm.RemoveAll(); err != nil {
				s.logger.Warn("清理 ASR 上传临时文件失败", "error", err)
			}
		}()
	}
	if parseErr != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(parseErr, &maxBytesErr) {
			Fail(c, http.StatusRequestEntityTooLarge, "asr_audio_too_large", "音频文件不能超过 64 MiB")
			return
		}
		Fail(c, http.StatusBadRequest, "invalid_asr_multipart", "无法解析音频上传请求")
		return
	}

	files := c.Request.MultipartForm.File
	fileHeaders := files["file"]
	if len(files) != 1 || len(fileHeaders) != 1 {
		Fail(c, http.StatusBadRequest, "invalid_asr_audio", "必须且只能上传一个音频文件")
		return
	}
	fileHeader := fileHeaders[0]
	if fileHeader.Size <= 0 {
		Fail(c, http.StatusBadRequest, "invalid_asr_audio", "音频文件不能为空")
		return
	}
	if fileHeader.Size > preprocessASRMaxAudioBytes {
		Fail(c, http.StatusRequestEntityTooLarge, "asr_audio_too_large", "音频文件不能超过 64 MiB")
		return
	}

	sourceInMs, err := requiredASRFormInt(c, "source_in_ms")
	if err != nil || sourceInMs < 0 {
		Fail(c, http.StatusBadRequest, "invalid_asr_source_range", "source_in_ms 必须是非负整数")
		return
	}
	sourceOutMs, err := requiredASRFormInt(c, "source_out_ms")
	if err != nil || sourceOutMs <= sourceInMs {
		Fail(c, http.StatusBadRequest, "invalid_asr_source_range", "source_out_ms 必须大于 source_in_ms")
		return
	}

	audio, err := fileHeader.Open()
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid_asr_audio", "无法读取上传的音频文件")
		return
	}
	defer audio.Close()

	client := modelgateway.NewFunASRClient(s.cfg.ASRBaseURL, s.cfg.ASRRequestTimeout)
	result, err := client.Transcribe(c.Request.Context(), modelgateway.FunASRTranscriptionInput{
		Filename:   fileHeader.Filename,
		Audio:      audio,
		DurationMs: sourceOutMs - sourceInMs,
	})
	if err != nil {
		s.handlePreprocessASRError(c, err)
		return
	}

	s.logger.Info(
		"预处理 ASR 转写完成",
		"duration", time.Since(startedAt),
		"selection_duration_ms", sourceOutMs-sourceInMs,
		"segment_count", len(result.Segments),
	)
	OK(c, gin.H{
		"text":          result.Text,
		"segments":      result.Segments,
		"source_in_ms":  sourceInMs,
		"source_out_ms": sourceOutMs,
		"time_base":     preprocessASRTimeBase,
	})
}

func (s *Server) handlePreprocessASRError(c *gin.Context, err error) {
	var asrErr *modelgateway.FunASRError
	if !errors.As(err, &asrErr) {
		Fail(c, http.StatusBadGateway, "asr_invalid_response", "语音识别服务返回异常")
		return
	}
	switch asrErr.Kind {
	case modelgateway.FunASRErrorTimeout:
		Fail(c, http.StatusGatewayTimeout, "asr_timeout", "语音识别服务请求超时，请稍后重试")
	case modelgateway.FunASRErrorUnavailable:
		Fail(c, http.StatusServiceUnavailable, "asr_unavailable", "语音识别服务暂不可用，请稍后重试")
	default:
		Fail(c, http.StatusBadGateway, "asr_invalid_response", "语音识别服务返回异常")
	}
}

func requiredASRFormInt(c *gin.Context, key string) (int, error) {
	value := strings.TrimSpace(c.PostForm(key))
	if value == "" {
		return 0, errors.New("missing form field")
	}
	return strconv.Atoi(value)
}
