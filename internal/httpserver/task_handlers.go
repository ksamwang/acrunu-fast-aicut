package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
)

func (s *Server) handleCreateTestTask(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	task := s.taskService.CreateTestTask(user.ID)
	if err := s.queueClient.EnqueueTestTask(task.ID); err != nil {
		_ = s.taskService.MarkFailed(task.ID, err.Error())
		Fail(c, http.StatusBadGateway, "queue_error", "failed to enqueue test task")
		return
	}

	Created(c, task)
}

func (s *Server) handleListTasks(c *gin.Context) {
	OK(c, s.taskService.ListTasks())
}
