package httpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func (s *Server) handleListSubtitleStylePresets(c *gin.Context) {
	presets, err := s.subtitleStylePresetService.List(c.Request.Context(), false)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "subtitle_style_error", "无法读取字幕样式")
		return
	}
	OK(c, presets)
}

func (s *Server) handleAdminListSubtitleStylePresets(c *gin.Context) {
	presets, err := s.subtitleStylePresetService.List(c.Request.Context(), true)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "subtitle_style_error", "无法读取字幕样式")
		return
	}
	OK(c, presets)
}

func (s *Server) handleCreateSubtitleStylePreset(c *gin.Context) {
	var input services.SubtitleStylePresetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_subtitle_style", "字幕样式格式不正确")
		return
	}
	preset, err := s.subtitleStylePresetService.Create(c.Request.Context(), input)
	if err != nil {
		handleSubtitleStyleError(c, err)
		return
	}
	Created(c, preset)
}

func (s *Server) handleUpdateSubtitleStylePreset(c *gin.Context) {
	var input services.SubtitleStylePresetInput
	if err := c.ShouldBindJSON(&input); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_subtitle_style", "字幕样式格式不正确")
		return
	}
	preset, err := s.subtitleStylePresetService.Update(c.Request.Context(), c.Param("presetID"), input)
	if err != nil {
		handleSubtitleStyleError(c, err)
		return
	}
	OK(c, preset)
}

func (s *Server) handleSetDefaultSubtitleStylePreset(c *gin.Context) {
	preset, err := s.subtitleStylePresetService.SetDefault(c.Request.Context(), c.Param("presetID"))
	if err != nil {
		handleSubtitleStyleError(c, err)
		return
	}
	OK(c, preset)
}

func (s *Server) handleDeleteSubtitleStylePreset(c *gin.Context) {
	if err := s.subtitleStylePresetService.Delete(c.Request.Context(), c.Param("presetID")); err != nil {
		handleSubtitleStyleError(c, err)
		return
	}
	OK(c, gin.H{"deleted": true})
}

func handleSubtitleStyleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrSubtitleStylePresetNotFound):
		Fail(c, http.StatusNotFound, "subtitle_style_not_found", "字幕样式不存在")
	case errors.Is(err, services.ErrSubtitleStylePresetConflict):
		Fail(c, http.StatusConflict, "subtitle_style_conflict", "字幕样式名称已存在")
	case errors.Is(err, services.ErrSubtitleStylePresetDefault):
		Fail(c, http.StatusConflict, "default_subtitle_style", "默认字幕样式不能停用或删除")
	case strings.Contains(err.Error(), "subtitle") || strings.Contains(err.Error(), "output_ratio"):
		Fail(c, http.StatusBadRequest, "invalid_subtitle_style", err.Error())
	default:
		Fail(c, http.StatusInternalServerError, "subtitle_style_error", "字幕样式操作失败")
	}
}
