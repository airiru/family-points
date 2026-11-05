// Package service：任务与投票。
package service

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"familypoints/server/internal/model"
	"familypoints/server/internal/notify"
	"familypoints/server/internal/repo"
)

// Task 是任务服务。
type Task struct {
	notifier *notify.Service
	tasks *repo.Task
}

// NewTask 构造。
func NewTask(t *repo.Task, n *notify.Service) *Task { return &Task{tasks: t, notifier: n} }

// TaskDetail 是任务详情（含提交与票数）。
type TaskDetail struct {
	Task        *model.Task             `json:"task"`
	Submissions []model.TaskSubmission  `json:"submissions"`
	Votes       map[uint64][]model.TaskVote `json:"votes"`
	Tally       map[uint64]VoteTally    `json:"tally"`
	Voters      int                     `json:"voters"`
}

// VoteTally 是某提交的票数汇总。
type VoteTally struct {
	Approve int `json:"approve"`
	Reject  int `json:"reject"`
	Need    int `json:"need"` // 通过所需同意票（> voters/2）
}

// Create 发布任务：任何成员都可以（含组长），管理员不行（无积分可冻）。
func (s *Task) Create(ctx context.Context, me *model.Member, title string, points int, dueAt *time.Time) (*model.Task, error) {
	title = strings.TrimSpace(title)
	if title == "" || utf8.RuneCountInString(title) > 128 {
		return nil, fmt.Errorf("%w：标题需 1–128 个字符", ErrInvalidInput)
	}
	if points <= 0 {
		return nil, fmt.Errorf("%w：悬赏分值必须是正整数（任务只做转移，不做惩罚）", ErrInvalidInput)
	}
	if dueAt != nil && dueAt.Before(time.Now()) {
		return nil, fmt.Errorf("%w：截止时间不能早于现在", ErrInvalidInput)
	}
	return s.tasks.Create(ctx, *me.FamilyID, me.ID, title, points, dueAt)
}

// List 列出本组任务（顺带把超时的提交置为 expired）。
func (s *Task) List(ctx context.Context, me *model.Member) ([]model.Task, error) {
	if err := s.tasks.ExpireDue(ctx, *me.FamilyID); err != nil {
		return nil, err
	}
	return s.tasks.List(ctx, *me.FamilyID)
}

// Detail 任务详情。
func (s *Task) Detail(ctx context.Context, me *model.Member, taskID uint64) (*TaskDetail, error) {
	if err := s.tasks.ExpireDue(ctx, *me.FamilyID); err != nil {
		return nil, err
	}
	t, err := s.tasks.ByID(ctx, *me.FamilyID, taskID)
	if err != nil {
		return nil, err
	}
	subs, err := s.tasks.Submissions(ctx, *me.FamilyID, taskID)
	if err != nil {
		return nil, err
	}
	votes := map[uint64][]model.TaskVote{}
	tally := map[uint64]VoteTally{}
	for _, sub := range subs {
		vs, err := s.tasks.Votes(ctx, *me.FamilyID, sub.ID)
		if err != nil {
			return nil, err
		}
		votes[sub.ID] = vs
		approve, reject := 0, 0
		for _, v := range vs {
			if v.Vote == "approve" {
				approve++
			} else {
				reject++
			}
		}
		tally[sub.ID] = VoteTally{Approve: approve, Reject: reject, Need: 0}
	}
	return &TaskDetail{Task: t, Submissions: subs, Votes: votes, Tally: tally}, nil
}

// Claim 接取任务。
func (s *Task) Claim(ctx context.Context, me *model.Member, taskID uint64) (*model.Task, error) {
	return s.tasks.Claim(ctx, *me.FamilyID, taskID, me.ID)
}

// Abandon 放弃接取。
func (s *Task) Abandon(ctx context.Context, me *model.Member, taskID uint64) error {
	return s.tasks.Abandon(ctx, *me.FamilyID, taskID, me.ID)
}

// Close 关闭任务（仅发布者）。
func (s *Task) Close(ctx context.Context, me *model.Member, taskID uint64) (*model.Task, error) {
	return s.tasks.Close(ctx, *me.FamilyID, taskID, me.ID)
}

// Submit 提交完成（仅接取者；note 必填 —— 无图片凭证，说明就是唯一凭证）。
func (s *Task) Submit(ctx context.Context, me *model.Member, taskID uint64, note string) (*model.TaskSubmission, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil, fmt.Errorf("%w：提交说明必填", ErrInvalidInput)
	}
	if utf8.RuneCountInString(note) > 255 {
		return nil, fmt.Errorf("%w：提交说明最多 255 个字符", ErrInvalidInput)
	}
	sub, err := s.tasks.Submit(ctx, *me.FamilyID, taskID, me.ID, note)
	if err != nil {
		return nil, err
	}
	// 旁路通知：失败不影响提交（notify 内部只记日志）
	if t, err := s.tasks.ByID(ctx, *me.FamilyID, taskID); err == nil {
		s.notifier.TaskSubmitted(ctx, *me.FamilyID, me.ID, t)
	}
	return sub, nil
}

// Vote 投票（同意 / 反对）。一人一票，不可改。
func (s *Task) Vote(ctx context.Context, me *model.Member, submissionID uint64, vote string) (*repo.VoteResult, error) {
	if vote != "approve" && vote != "reject" {
		return nil, fmt.Errorf("%w：vote 只能是 approve 或 reject", ErrInvalidInput)
	}
	res, err := s.tasks.Vote(ctx, *me.FamilyID, submissionID, me.ID, vote)
	if err != nil {
		return nil, err
	}
	if res.Status == "approved" || res.Status == "rejected" {
		if t, err := s.tasks.ByID(ctx, *me.FamilyID, res.TaskID); err == nil {
			s.notifier.TaskDecided(ctx, *me.FamilyID, res.CompleterID, res.TaskID, t.Title, res.Status)
		}
	}
	return res, nil
}
