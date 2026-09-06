package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	hosttypes "github.com/QuantumNous/new-api/types"
)

type taskUsageLogKey struct {
	UserID int
	TaskID string
}

type taskUsageLogOther struct {
	IsTask bool   `json:"is_task"`
	TaskID string `json:"task_id"`
}

// EnrichTaskUsageLogs attaches safe, normalized task usage to log responses
// without mutating append-oriented log storage.
func EnrichTaskUsageLogs(logs []*model.Log) error {
	keys := make(map[taskUsageLogKey]struct{})
	taskIDs := make(map[string]struct{})
	userIDs := make(map[int]struct{})
	for _, log := range logs {
		if log == nil || log.Other == "" {
			continue
		}
		var other taskUsageLogOther
		if err := common.UnmarshalJsonStr(log.Other, &other); err != nil || !other.IsTask || other.TaskID == "" {
			continue
		}
		key := taskUsageLogKey{UserID: log.UserId, TaskID: other.TaskID}
		keys[key] = struct{}{}
		taskIDs[other.TaskID] = struct{}{}
		userIDs[log.UserId] = struct{}{}
	}
	if len(keys) == 0 {
		return nil
	}

	taskIDList := make([]string, 0, len(taskIDs))
	for taskID := range taskIDs {
		taskIDList = append(taskIDList, taskID)
	}
	userIDList := make([]int, 0, len(userIDs))
	for userID := range userIDs {
		userIDList = append(userIDList, userID)
	}
	tasks, err := model.GetTasksByTaskIDsAndUsers(taskIDList, userIDList)
	if err != nil {
		return err
	}
	usageByKey := make(map[taskUsageLogKey]*hosttypes.TaskUsage, len(tasks))
	for _, task := range tasks {
		if task == nil || task.Usage == nil {
			continue
		}
		usageByKey[taskUsageLogKey{UserID: task.UserId, TaskID: task.TaskID}] = task.Usage
	}

	for _, log := range logs {
		if log == nil || log.Other == "" {
			continue
		}
		var other map[string]interface{}
		if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
			continue
		}
		isTask, _ := other["is_task"].(bool)
		if !isTask {
			continue
		}
		taskID, _ := other["task_id"].(string)
		usage := usageByKey[taskUsageLogKey{UserID: log.UserId, TaskID: taskID}]
		if usage == nil {
			continue
		}
		other["task_usage"] = usage
		log.Other = common.MapToJsonStr(other)
	}
	return nil
}
