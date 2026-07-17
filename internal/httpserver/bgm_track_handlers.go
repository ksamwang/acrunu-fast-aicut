package httpserver

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

const maxBGMUploadRequestBytes int64 = 102 << 20

func (s *Server) handleListBGMTracks(c *gin.Context) {
	includeInactive, _ := strconv.ParseBool(c.Query("include_inactive"))
	tracks, err := s.bgmTrackService.List(c.Request.Context(), includeInactive)
	if err != nil {
		handleBGMTrackError(c, err)
		return
	}
	OK(c, tracks)
}

func (s *Server) handleCreateBGMTrack(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBGMUploadRequestBytes)
	if err := c.Request.ParseMultipartForm(16 << 20); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_bgm_track", "音乐上传格式不正确或文件过大")
		return
	}
	file, header, err := c.Request.FormFile("audio")
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid_bgm_track", "请选择音乐文件")
		return
	}
	defer file.Close()
	input, err := parseBGMTrackForm(c.Request.MultipartForm, header, file)
	if err != nil {
		Fail(c, http.StatusBadRequest, "invalid_bgm_track", err.Error())
		return
	}
	track, err := s.bgmTrackService.Create(c.Request.Context(), input, user.ID)
	if err != nil {
		handleBGMTrackError(c, err)
		return
	}
	Created(c, track)
}

func (s *Server) handleUpdateBGMTrack(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	var input services.BGMTrackInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_bgm_track", "音乐信息格式不正确")
		return
	}
	track, err := s.bgmTrackService.Update(c.Request.Context(), c.Param("trackID"), input, user.ID)
	if err != nil {
		handleBGMTrackError(c, err)
		return
	}
	OK(c, track)
}

func (s *Server) handleArchiveBGMTrack(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}
	track, err := s.bgmTrackService.Archive(c.Request.Context(), c.Param("trackID"), user.ID)
	if err != nil {
		handleBGMTrackError(c, err)
		return
	}
	OK(c, track)
}

func parseBGMTrackForm(form *multipart.Form, header *multipart.FileHeader, file multipart.File) (services.BGMTrackUpload, error) {
	value := func(key string) string {
		if form == nil || len(form.Value[key]) == 0 {
			return ""
		}
		return form.Value[key][0]
	}
	bpm := 0
	if raw := strings.TrimSpace(value("bpm")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return services.BGMTrackUpload{}, errors.New("BPM 必须是整数")
		}
		bpm = parsed
	}
	tags := []string{}
	if raw := strings.TrimSpace(value("tags_json")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			return services.BGMTrackUpload{}, errors.New("标签格式不正确")
		}
	}
	return services.BGMTrackUpload{
		BGMTrackInput: services.BGMTrackInput{
			Name: value("name"), BPM: bpm, Mood: value("mood"), Tags: tags, Status: value("status"),
		},
		FileName: header.Filename,
		MimeType: header.Header.Get("Content-Type"),
		Reader:   file,
	}, nil
}

func handleBGMTrackError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrBGMTrackNotFound):
		Fail(c, http.StatusNotFound, "bgm_track_not_found", "背景音乐不存在")
	case errors.Is(err, services.ErrBGMTrackUnavailable):
		Fail(c, http.StatusConflict, "bgm_track_unavailable", "没有可用的背景音乐")
	case errors.Is(err, services.ErrBGMTrackInvalid):
		Fail(c, http.StatusBadRequest, "invalid_bgm_track", err.Error())
	default:
		Fail(c, http.StatusInternalServerError, "bgm_track_error", "背景音乐处理失败")
	}
}
