// Package repo：任务与投票（积分转移）。
//
// 经济本质（16 条）：**任务是转移，不是生成**。
// 发布时从发布者冻结，审核通过时冻结释放并转给完成者，家庭总积分不变。
package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"familypoints/server/internal/model"
)

// Task 封装任务 SQL。
type Task struct{ db *sqlx.DB }

// NewTask 构造。
func NewTask(db *sqlx.DB) *Task { return &Task{db: db} }

// VoteWindow 是投票期（20.3：提交后 24 小时，或任务截止时间取较早者）。
const VoteWindow = 24 * time.Hour

// Create 发布任务：校验余额 → 冻结 → 写任务 → 写一条 0 分说明流水。
//
// 冻结是「同一成员内部的额度调整」（score → frozen_score），**总资产不变**，
// 所以不产生 ±N 的流水；真正转移发生在审核通过时。
func (r *Task) Create(ctx context.Context, familyID, publisherID uint64, title string, points int, dueAt *time.Time) (*model.Task, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// 有账号成员仅 1 人时禁止发布（除发布者外无人可投，永远无法过半）
	var memberCount int
	if err := tx.GetContext(ctx, &memberCount,
		`SELECT COUNT(*) FROM members WHERE family_id = ?`, familyID); err != nil {
		return nil, err
	}
	if memberCount < 2 {
		return nil, ErrTooFewMembers
	}

	var m struct {
		Name  string `db:"name"`
		Score int    `db:"score"`
	}
	if err := tx.GetContext(ctx, &m,
		`SELECT name, score FROM members WHERE id = ? AND family_id = ? FOR UPDATE`,
		publisherID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if m.Score < points {
		return nil, ErrInsufficient
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score - ?, frozen_score = frozen_score + ?
		  WHERE id = ? AND family_id = ?`, points, points, publisherID, familyID); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO tasks (family_id, title, points, publisher_id, escrow_points, due_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, familyID, title, points, publisherID, points, dueAt)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 0 分说明流水：冻结不是资产变动，但要让流水看得出「钱去哪了」
	now := time.Now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO records (family_id, member_id, member_name, title, points,
		                      occurred_at, added_at, source)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		familyID, publisherID, m.Name, fmt.Sprintf("发布任务：%s（冻结 %d 分）", title, points),
		now, now, model.SourceTask); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &model.Task{
		ID: uint64(id), FamilyID: familyID, Title: title, Points: points,
		PublisherID: publisherID, EscrowPoints: points, DueAt: dueAt,
		Status: model.TaskOpen, CreatedAt: now,
	}, nil
}

// List 列出本组任务（新→旧）。
func (r *Task) List(ctx context.Context, familyID uint64) ([]model.Task, error) {
	var list []model.Task
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, title, points, publisher_id, escrow_points, assignee_id,
		        completed_by, due_at, repeat_rule, status, created_at
		   FROM tasks WHERE family_id = ? ORDER BY id DESC`, familyID)
	return list, err
}

// ByID 取任务（带 family_id，防跨组）。
func (r *Task) ByID(ctx context.Context, familyID, id uint64) (*model.Task, error) {
	var t model.Task
	err := r.db.GetContext(ctx, &t,
		`SELECT id, family_id, title, points, publisher_id, escrow_points, assignee_id,
		        completed_by, due_at, repeat_rule, status, created_at
		   FROM tasks WHERE id = ? AND family_id = ?`, id, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Claim 接取任务（先到先得）。发布者不能接自己的任务。
func (r *Task) Claim(ctx context.Context, familyID, taskID, memberID uint64) (*model.Task, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var t model.Task
	if err := tx.GetContext(ctx, &t,
		`SELECT id, family_id, title, points, publisher_id, escrow_points, assignee_id,
		        completed_by, due_at, repeat_rule, status, created_at
		   FROM tasks WHERE id = ? AND family_id = ? FOR UPDATE`, taskID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t.PublisherID == memberID {
		return nil, ErrSelfTask
	}
	if t.Status != model.TaskOpen || t.AssigneeID != nil {
		return nil, ErrTaskNotOpen
	}

	// 条件更新：并发接取时只有一个能成功
	res, err := tx.ExecContext(ctx,
		`UPDATE tasks SET assignee_id = ?, status = ? WHERE id = ? AND status = ? AND assignee_id IS NULL`,
		memberID, model.TaskTaken, taskID, model.TaskOpen)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrTaskNotOpen
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t.AssigneeID = &memberID
	t.Status = model.TaskTaken
	return &t, nil
}

// Abandon 放弃接取：任务回到 open，其他人可再接。
func (r *Task) Abandon(ctx context.Context, familyID, taskID, memberID uint64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET assignee_id = NULL, status = ?
		  WHERE id = ? AND family_id = ? AND assignee_id = ? AND status = ?`,
		model.TaskOpen, taskID, familyID, memberID, model.TaskTaken)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTaskNotOpen
	}
	return nil
}

// Close 发布者关闭任务：冻结原路退回（frozen → score），总资产不变。
func (r *Task) Close(ctx context.Context, familyID, taskID, publisherID uint64) (*model.Task, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var t model.Task
	if err := tx.GetContext(ctx, &t,
		`SELECT id, family_id, title, points, publisher_id, escrow_points, assignee_id,
		        completed_by, due_at, repeat_rule, status, created_at
		   FROM tasks WHERE id = ? AND family_id = ? FOR UPDATE`, taskID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t.PublisherID != publisherID {
		return nil, ErrNotPublisher
	}
	if t.Status == model.TaskClosed {
		return nil, ErrTaskNotOpen
	}

	var m struct {
		Name string `db:"name"`
	}
	if err := tx.GetContext(ctx, &m,
		`SELECT name FROM members WHERE id = ? AND family_id = ?`, publisherID, familyID); err != nil {
		return nil, err
	}

	// 退回冻结：frozen_score → score
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET frozen_score = frozen_score - ?, score = score + ?
		  WHERE id = ? AND family_id = ?`, t.EscrowPoints, t.EscrowPoints, publisherID, familyID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE tasks SET status = ?, escrow_points = 0 WHERE id = ? AND family_id = ?`,
		model.TaskClosed, taskID, familyID); err != nil {
		return nil, err
	}

	now := time.Now()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO records (family_id, member_id, member_name, title, points,
		                      occurred_at, added_at, source)
		 VALUES (?, ?, ?, ?, 0, ?, ?, ?)`,
		familyID, publisherID, m.Name,
		fmt.Sprintf("关闭任务：%s（退回冻结 %d 分）", t.Title, t.EscrowPoints),
		now, now, model.SourceTask); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	t.Status = model.TaskClosed
	t.EscrowPoints = 0
	return &t, nil
}

// Submit 提交完成。仅接取者本人，note 必填，驳回后最多 3 次。
func (r *Task) Submit(ctx context.Context, familyID, taskID, memberID uint64, note string) (*model.TaskSubmission, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var t model.Task
	if err := tx.GetContext(ctx, &t,
		`SELECT id, family_id, title, points, publisher_id, escrow_points, assignee_id,
		        completed_by, due_at, repeat_rule, status, created_at
		   FROM tasks WHERE id = ? AND family_id = ? FOR UPDATE`, taskID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if t.AssigneeID == nil || *t.AssigneeID != memberID {
		return nil, ErrNotAssignee
	}
	if t.Status != model.TaskTaken {
		return nil, ErrTaskNotOpen
	}

	// 重提上限：同一 task + member 的 rejected 数达到 3 就锁定（expired 不计入）
	var rejected int
	if err := tx.GetContext(ctx, &rejected,
		`SELECT COUNT(*) FROM task_submissions
		  WHERE task_id = ? AND member_id = ? AND family_id = ? AND status = ?`,
		taskID, memberID, familyID, model.SubmissionRejected); err != nil {
		return nil, err
	}
	if rejected >= 3 {
		return nil, ErrResubmitLimit
	}

	// 同一时刻只允许一条待审
	var pending int
	if err := tx.GetContext(ctx, &pending,
		`SELECT COUNT(*) FROM task_submissions
		  WHERE task_id = ? AND member_id = ? AND family_id = ? AND status = ?`,
		taskID, memberID, familyID, model.SubmissionSubmitted); err != nil {
		return nil, err
	}
	if pending > 0 {
		return nil, ErrAlreadySubmitted
	}

	// 投票期：24 小时与截止时间取较早者
	deadline := time.Now().Add(VoteWindow)
	if t.DueAt != nil && t.DueAt.Before(deadline) {
		deadline = *t.DueAt
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO task_submissions (family_id, task_id, member_id, note, vote_deadline)
		 VALUES (?, ?, ?, ?, ?)`, familyID, taskID, memberID, note, deadline)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &model.TaskSubmission{
		ID: uint64(id), FamilyID: familyID, TaskID: taskID, MemberID: memberID,
		Note: note, Status: model.SubmissionSubmitted, VoteDeadline: &deadline, CreatedAt: time.Now(),
	}, nil
}

// Submissions 列出某任务的全部提交。
func (r *Task) Submissions(ctx context.Context, familyID, taskID uint64) ([]model.TaskSubmission, error) {
	var list []model.TaskSubmission
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, task_id, member_id, note, status, vote_deadline,
		        review_note, reviewed_at, record_id, created_at
		   FROM task_submissions WHERE family_id = ? AND task_id = ? ORDER BY id DESC`,
		familyID, taskID)
	return list, err
}

// SubmissionByID 取提交（带 family_id）。
func (r *Task) SubmissionByID(ctx context.Context, familyID, id uint64) (*model.TaskSubmission, error) {
	var s model.TaskSubmission
	err := r.db.GetContext(ctx, &s,
		`SELECT id, family_id, task_id, member_id, note, status, vote_deadline,
		        review_note, reviewed_at, record_id, created_at
		   FROM task_submissions WHERE id = ? AND family_id = ?`, id, familyID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Votes 列出某提交的票。
func (r *Task) Votes(ctx context.Context, familyID, submissionID uint64) ([]model.TaskVote, error) {
	var list []model.TaskVote
	err := r.db.SelectContext(ctx, &list,
		`SELECT id, family_id, submission_id, voter_id, vote, created_at
		   FROM task_votes WHERE family_id = ? AND submission_id = ? ORDER BY id`,
		familyID, submissionID)
	return list, err
}

// VoteResult 是一次投票后的状态。
type VoteResult struct {
	Status   string // submitted / approved / rejected / expired
	Approve  int
	Reject   int
	Voters   int // 有投票权人数（成员数 − 发布者）
	Transferred bool
}

// Vote 投票并即时判定（过半即通过，反对足以否决即驳回）。
//
// 判定口径（20.3）：通过 = 同意票 > 有投票权人数 ÷ 2。
// 提前驳回 = 同意票 + 剩余未投票 ≤ 半数（同意已不可能过半）。
func (r *Task) Vote(ctx context.Context, familyID, submissionID, voterID uint64, vote string) (*VoteResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var s model.TaskSubmission
	if err := tx.GetContext(ctx, &s,
		`SELECT id, family_id, task_id, member_id, note, status, vote_deadline,
		        review_note, reviewed_at, record_id, created_at
		   FROM task_submissions WHERE id = ? AND family_id = ? FOR UPDATE`,
		submissionID, familyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if s.Status != model.SubmissionSubmitted {
		return nil, ErrNotVotable
	}
	if s.VoteDeadline != nil && time.Now().After(*s.VoteDeadline) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE task_submissions SET status = ? WHERE id = ? AND family_id = ?`,
			model.SubmissionExpired, submissionID, familyID); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &VoteResult{Status: model.SubmissionExpired}, ErrVoteExpired
	}

	var t model.Task
	if err := tx.GetContext(ctx, &t,
		`SELECT id, family_id, title, points, publisher_id, escrow_points, assignee_id,
		        completed_by, due_at, repeat_rule, status, created_at
		   FROM tasks WHERE id = ? AND family_id = ?`, s.TaskID, familyID); err != nil {
		return nil, err
	}
	if t.PublisherID == voterID {
		return nil, ErrPublisherNoVote
	}

	// 一人一票：唯一索引兜底，这里先查一次给出明确错误
	var voted int
	if err := tx.GetContext(ctx, &voted,
		`SELECT COUNT(*) FROM task_votes WHERE submission_id = ? AND voter_id = ?`,
		submissionID, voterID); err != nil {
		return nil, err
	}
	if voted > 0 {
		return nil, ErrAlreadyVoted
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO task_votes (family_id, submission_id, voter_id, vote) VALUES (?, ?, ?, ?)`,
		familyID, submissionID, voterID, vote); err != nil {
		return nil, err
	}

	// 票数统计
	var approve, reject int
	if err := tx.GetContext(ctx, &approve,
		`SELECT COUNT(*) FROM task_votes WHERE submission_id = ? AND vote = 'approve'`,
		submissionID); err != nil {
		return nil, err
	}
	if err := tx.GetContext(ctx, &reject,
		`SELECT COUNT(*) FROM task_votes WHERE submission_id = ? AND vote = 'reject'`,
		submissionID); err != nil {
		return nil, err
	}

	// 分母 = 家庭组成员数 − 1（发布者）
	var memberCount int
	if err := tx.GetContext(ctx, &memberCount,
		`SELECT COUNT(*) FROM members WHERE family_id = ?`, familyID); err != nil {
		return nil, err
	}
	voters := memberCount - 1
	if voters < 1 {
		voters = 1
	}
	result := &VoteResult{Status: model.SubmissionSubmitted, Approve: approve, Reject: reject, Voters: voters}

	// 通过：同意票 > 半数
	if approve*2 > voters {
		if err := approveSubmission(ctx, tx, familyID, &s, &t); err != nil {
			return nil, err
		}
		result.Status = model.SubmissionApproved
		result.Transferred = true
	} else if approve+(voters-approve-reject) <= voters/2 {
		// 提前驳回：剩余票全给同意也过不了
		if _, err := tx.ExecContext(ctx,
			`UPDATE task_submissions SET status = ?, reviewed_at = NOW(3) WHERE id = ? AND family_id = ?`,
			model.SubmissionRejected, submissionID, familyID); err != nil {
			return nil, err
		}
		result.Status = model.SubmissionRejected
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// approveSubmission 通过审核：冻结释放 → 转给完成者 → 写两条流水 → 关闭任务。
func approveSubmission(ctx context.Context, tx *sqlx.Tx, familyID uint64, s *model.TaskSubmission, t *model.Task) error {
	var publisher, completer struct {
		ID   uint64 `db:"id"`
		Name string `db:"name"`
	}
	if err := tx.GetContext(ctx, &publisher,
		`SELECT id, name FROM members WHERE id = ? AND family_id = ?`, t.PublisherID, familyID); err != nil {
		return err
	}
	if err := tx.GetContext(ctx, &completer,
		`SELECT id, name FROM members WHERE id = ? AND family_id = ?`, s.MemberID, familyID); err != nil {
		return err
	}

	// 幂等：条件更新，受影响行数 0 说明已被处理
	res, err := tx.ExecContext(ctx,
		`UPDATE task_submissions SET status = ?, reviewed_at = NOW(3)
		  WHERE id = ? AND family_id = ? AND status = ?`,
		model.SubmissionApproved, s.ID, familyID, model.SubmissionSubmitted)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotVotable
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET frozen_score = frozen_score - ? WHERE id = ? AND family_id = ?`,
		t.Points, publisher.ID, familyID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE members SET score = score + ? WHERE id = ? AND family_id = ?`,
		t.Points, completer.ID, familyID); err != nil {
		return err
	}

	now := time.Now()
	for _, row := range []struct {
		memberID uint64
		name     string
		title    string
		points   int
	}{
		{publisher.ID, publisher.Name, fmt.Sprintf("任务悬赏：%s（转给 %s）", t.Title, completer.Name), -t.Points},
		{completer.ID, completer.Name, fmt.Sprintf("完成任务：%s（来自 %s）", t.Title, publisher.Name), t.Points},
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO records (family_id, member_id, member_name, title, points,
			                      occurred_at, added_at, source)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			familyID, row.memberID, row.name, row.title, row.points, now, now, model.SourceTask); err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE tasks SET status = ?, completed_by = ?, escrow_points = 0 WHERE id = ? AND family_id = ?`,
		model.TaskClosed, completer.ID, t.ID, familyID)
	return err
}

// ExpireDue 把已过投票期且仍待审的提交置为 expired（惰性清扫）。
//
// 没有定时任务：在「列表/详情/投票」这些读路径上顺手清扫，
// 家庭场景量级下足够，且不会因为漏跑 cron 而永远停在 submitted。
func (r *Task) ExpireDue(ctx context.Context, familyID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE task_submissions SET status = ?
		  WHERE family_id = ? AND status = ? AND vote_deadline IS NOT NULL AND vote_deadline < NOW(3)`,
		model.SubmissionExpired, familyID, model.SubmissionSubmitted)
	return err
}

// ErrTooFewMembers 等业务错误由 repo 抛给 service 再映射。
var (
	ErrTooFewMembers    = errors.New("家庭组成员不足，无法发布任务")
	ErrSelfTask         = errors.New("不能接取自己发布的任务")
	ErrTaskNotOpen      = errors.New("任务状态不允许该操作")
	ErrNotPublisher     = errors.New("只有发布者可以关闭任务")
	ErrNotAssignee      = errors.New("只有接取者可以提交")
	ErrResubmitLimit    = errors.New("驳回次数已达上限")
	ErrAlreadySubmitted = errors.New("已有待审核的提交")
	ErrNotVotable       = errors.New("该提交当前不可投票")
	ErrVoteExpired      = errors.New("投票已超时")
	ErrPublisherNoVote  = errors.New("发布者没有投票权")
	ErrAlreadyVoted     = errors.New("你已经投过票了")
)
