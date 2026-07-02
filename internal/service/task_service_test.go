package service

import (
	"errors"
	"testing"
	"time"

	models "emplacc-api/internal/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskMoveRejectsClosedTargetStatusWithoutMutation(t *testing.T) {
	boardID := uuid.New()
	taskID := uuid.New()
	currentStatusID := uuid.New()
	closedStatusID := uuid.New()
	closed := false
	open := true
	repo := &fakeTaskRepository{
		tasks: map[uuid.UUID]*models.Task{
			taskID: {ID: taskID, StatusID: currentStatusID},
		},
		statuses: map[uuid.UUID]*models.Status{
			currentStatusID: {ID: currentStatusID, BoardID: boardID, IsOpen: &open},
			closedStatusID:  {ID: closedStatusID, BoardID: boardID, IsOpen: &closed},
		},
	}
	svc := NewTaskService(repo, nil)

	statuses, err := svc.TaskMoveFunc(taskID, closedStatusID)

	require.ErrorIs(t, err, ErrTaskMoveCloseGateRequired)
	require.Nil(t, statuses)
	require.False(t, repo.updateTaskStatusCalled)
	require.Equal(t, currentStatusID, repo.tasks[taskID].StatusID)
}

func TestTaskMoveUpdatesOpenTargetStatus(t *testing.T) {
	boardID := uuid.New()
	taskID := uuid.New()
	currentStatusID := uuid.New()
	openStatusID := uuid.New()
	open := true
	repo := &fakeTaskRepository{
		tasks: map[uuid.UUID]*models.Task{
			taskID: {ID: taskID, StatusID: currentStatusID},
		},
		statuses: map[uuid.UUID]*models.Status{
			currentStatusID: {ID: currentStatusID, BoardID: boardID, IsOpen: &open},
			openStatusID:    {ID: openStatusID, BoardID: boardID, IsOpen: &open},
		},
		statusesByBoardID: map[uuid.UUID][]models.Status{
			boardID: {{ID: currentStatusID, BoardID: boardID}, {ID: openStatusID, BoardID: boardID}},
		},
	}
	svc := NewTaskService(repo, nil)

	statuses, err := svc.TaskMoveFunc(taskID, openStatusID)

	require.NoError(t, err)
	require.True(t, repo.updateTaskStatusCalled)
	require.Equal(t, taskID, repo.updatedTaskID)
	require.Equal(t, openStatusID, repo.updatedStatusID)
	require.Equal(t, openStatusID, repo.tasks[taskID].StatusID)
	require.Len(t, statuses, 2)
}

type fakeTaskRepository struct {
	tasks                  map[uuid.UUID]*models.Task
	statuses               map[uuid.UUID]*models.Status
	statusesByBoardID      map[uuid.UUID][]models.Status
	updateTaskStatusCalled bool
	updatedTaskID          uuid.UUID
	updatedStatusID        uuid.UUID
}

func (r *fakeTaskRepository) GetAllTasks(limit, offset int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetTaskByID(taskID uuid.UUID) (*models.Task, error) {
	task, ok := r.tasks[taskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	cp := *task
	return &cp, nil
}

func (r *fakeTaskRepository) GetTasksByProjectID(projectID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (r *fakeTaskRepository) CreateTask(task models.Task) error {
	return errors.New("not implemented")
}

func (r *fakeTaskRepository) UpdateTask(taskID uuid.UUID, updates map[string]interface{}) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *fakeTaskRepository) DeleteTask(taskID uuid.UUID) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetTasksByUserId(userID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetStatusByID(statusID uuid.UUID) (*models.Status, error) {
	status, ok := r.statuses[statusID]
	if !ok {
		return nil, errors.New("status not found")
	}
	cp := *status
	return &cp, nil
}

func (r *fakeTaskRepository) GetStatusesByBoardID(boardID uuid.UUID) ([]models.Status, error) {
	return append([]models.Status(nil), r.statusesByBoardID[boardID]...), nil
}

func (r *fakeTaskRepository) UpdateTaskStatus(taskID uuid.UUID, toStatusID uuid.UUID, updatedAt time.Time) error {
	r.updateTaskStatusCalled = true
	r.updatedTaskID = taskID
	r.updatedStatusID = toStatusID
	r.tasks[taskID].StatusID = toStatusID
	r.tasks[taskID].UpdatedAt = &updatedAt
	return nil
}

func (r *fakeTaskRepository) UserExists(userID uuid.UUID) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *fakeTaskRepository) StatusExists(statusID uuid.UUID) (bool, error) {
	return false, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetTasksByUserIDAndProjectID(userID, projectID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetActiveTasksByUserId(userID uuid.UUID, limit, offset int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetTaskBoardAndProjectIDs(taskID uuid.UUID) (boardId, projectId uuid.UUID, err error) {
	return uuid.Nil, uuid.Nil, errors.New("not implemented")
}

func (r *fakeTaskRepository) GetAllActiveTasks() ([]models.Task, error) {
	return nil, errors.New("not implemented")
}

func (r *fakeTaskRepository) SearchTasks(query, userID string, limit, offset int) ([]models.Task, int64, error) {
	return nil, 0, errors.New("not implemented")
}
