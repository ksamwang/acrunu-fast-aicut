package httpserver

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

func (s *Server) handleMetrics(c *gin.Context) {
	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	lines := []string{
		"# HELP aicut_http_metrics_enabled Whether the metrics endpoint is enabled.",
		"# TYPE aicut_http_metrics_enabled gauge",
		`aicut_http_metrics_enabled{component="api"} 1`,
	}

	if s.taskService != nil {
		tasks, err := s.taskService.ListTasks(c.Request.Context(), services.TaskFilters{})
		if err != nil {
			Fail(c, http.StatusInternalServerError, "metrics_error", "failed to build metrics")
			return
		}

		lines = append(lines,
			"# HELP aicut_generation_tasks_total Current generation task count by type and status.",
			"# TYPE aicut_generation_tasks_total gauge",
		)

		counts := map[string]int{}
		for _, task := range tasks {
			key := task.TaskType + "\x00" + task.Status
			counts[key]++
		}

		keys := make([]string, 0, len(counts))
		for key := range counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			parts := strings.SplitN(key, "\x00", 2)
			taskType := prometheusLabelValue(parts[0])
			status := ""
			if len(parts) > 1 {
				status = prometheusLabelValue(parts[1])
			}
			lines = append(lines, fmt.Sprintf(
				`aicut_generation_tasks_total{task_type="%s",status="%s"} %d`,
				taskType,
				status,
				counts[key],
			))
		}
	}

	c.String(http.StatusOK, strings.Join(lines, "\n")+"\n")
}

func prometheusLabelValue(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)
	return replacer.Replace(value)
}
