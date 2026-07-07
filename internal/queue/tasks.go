package queue

const TypeTestTask = "test:task"

type TestTaskPayload struct {
	TaskID string `json:"task_id"`
}
