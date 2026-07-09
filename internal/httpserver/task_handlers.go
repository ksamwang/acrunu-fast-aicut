package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/auth"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func (s *Server) handleCreateTestTask(c *gin.Context) {
	user, ok := auth.CurrentUser(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "unauthorized", "missing user context")
		return
	}

	task, err := s.taskService.CreateTestTask(c.Request.Context(), user.ID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "task_error", "failed to create test task")
		return
	}
	if err := s.queueClient.EnqueueTestTask(task.ID); err != nil {
		_ = s.taskService.MarkFailed(c.Request.Context(), task.ID, err.Error())
		Fail(c, http.StatusBadGateway, "queue_error", "failed to enqueue test task")
		return
	}

	Created(c, task)
}

func (s *Server) handleListTasks(c *gin.Context) {
	tasks, err := s.taskService.ListTasks(c.Request.Context(), services.TaskFilters{
		TaskType: c.Query("task_type"),
		Status:   c.Query("status"),
	})
	if err != nil {
		Fail(c, http.StatusInternalServerError, "task_error", "failed to list tasks")
		return
	}
	OK(c, tasks)
}

func (s *Server) handleGetTask(c *gin.Context) {
	task, err := s.taskService.GetTask(c.Request.Context(), c.Param("taskID"))
	if err != nil {
		statusCode := http.StatusInternalServerError
		errorCode := "task_error"
		message := "failed to get task"
		if err == services.ErrTaskNotFound {
			statusCode = http.StatusNotFound
			errorCode = "not_found"
			message = "task not found"
		}
		Fail(c, statusCode, errorCode, message)
		return
	}
	OK(c, task)
}
