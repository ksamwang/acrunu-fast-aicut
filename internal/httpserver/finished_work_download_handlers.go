package httpserver

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type createFinishedWorkDownloadRequest struct {
	WorkIDs []string `json:"work_ids"`
}

type finishedWorkDownloadResponse struct {
	DownloadURL string    `json:"download_url"`
	FileName    string    `json:"file_name"`
	FileCount   int       `json:"file_count"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Server) handleCreateFinishedWorkDownload(c *gin.Context) {
	var request createFinishedWorkDownloadRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_finished_work_download", "请选择需要下载的成品")
		return
	}
	batch, err := s.workDownloadService.Create(c.Request.Context(), request.WorkIDs)
	if err != nil {
		handleFinishedWorkDownloadError(c, err)
		return
	}
	Created(c, finishedWorkDownloadResponse{
		DownloadURL: "/api/workbench/download-batches/" + url.PathEscape(batch.Token),
		FileName:    batch.FileName, FileCount: batch.FileCount, ExpiresAt: batch.ExpiresAt,
	})
}

func (s *Server) handleDownloadFinishedWorks(c *gin.Context) {
	batch, files, err := s.workDownloadService.Consume(c.Param("token"))
	if err != nil {
		handleFinishedWorkDownloadError(c, err)
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"AICut-finished-works.zip\"; filename*=UTF-8''%s", url.PathEscape(batch.FileName)))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	archive := zip.NewWriter(c.Writer)
	for _, item := range files {
		file, openErr := os.Open(item.Path)
		if openErr != nil {
			s.logger.Error("open finished work for download failed", "path", item.Path, "error", openErr)
			_ = archive.Close()
			return
		}
		header := &zip.FileHeader{Name: item.FileName, Method: zip.Store}
		header.SetModTime(item.ModTime)
		entry, createErr := archive.CreateHeader(header)
		if createErr == nil {
			_, createErr = io.Copy(entry, file)
		}
		_ = file.Close()
		if createErr != nil {
			s.logger.Error("stream finished work download failed", "file", item.FileName, "error", createErr)
			_ = archive.Close()
			return
		}
	}
	if err := archive.Close(); err != nil {
		s.logger.Error("close finished work download failed", "error", err)
	}
}

func handleFinishedWorkDownloadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrFinishedWorkDownloadInvalid):
		Fail(c, http.StatusBadRequest, "invalid_finished_work_download", "每次请选择 1 至 50 个成品")
	case errors.Is(err, services.ErrFinishedWorkDownloadToken):
		Fail(c, http.StatusNotFound, "finished_work_download_expired", "下载链接不存在或已过期")
	case errors.Is(err, services.ErrFinishedWorkDownloadUnavailable):
		Fail(c, http.StatusConflict, "finished_work_download_unavailable", "所选成品尚未完成或文件不可用")
	default:
		Fail(c, http.StatusInternalServerError, "finished_work_download_error", "创建批量下载失败")
	}
}
