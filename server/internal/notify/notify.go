// Package notify 封装站内通知的「业务语义」。
//
// 为什么单独一层：通知是旁路——失败只记日志，绝不阻断业务。
// 各个 service 里若各自拼 title/body，措辞会漂；集中在这里便于统一。
package notify

import (
	"context"
	"log"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// Service 发通知。
type Service struct {
	n *repo.Notification
	f *repo.Family
}

// New 构造。
func New(n *repo.Notification, f *repo.Family) *Service {
	return &Service{n: n, f: f}
}

// 通知类型（前端据此决定跳转）。
const (
	TypeTaskSubmitted = "task_submitted" // 有人提交任务，等你投票
	TypeTaskDecided   = "task_decided"   // 你的任务被通过/驳回
	TypeCouponPending = "coupon_pending" // 有人发起券核销，等你投票
	TypeCouponDecided = "coupon_decided" // 你的券核销通过/驳回
	TypeTransferIn    = "transfer_in"    // 收到积分
	TypeKicked        = "kicked"         // 被移出家庭组
)

// one 发给单个成员。
func (s *Service) one(ctx context.Context, familyID, memberID uint64, typ, title, body, refType string, refID uint64) {
	if s == nil || s.n == nil || memberID == 0 {
		return
	}
	var rid *uint64
	if refID != 0 {
		rid = &refID
	}
	if err := s.n.Notify(ctx, familyID, memberID, typ, title, body, refType, rid); err != nil {
		log.Printf("WARN 写通知失败 type=%s member=%d: %v", typ, memberID, err)
	}
}

// familyExcept 发给全组除某人以外的成员（用于「等你们投票」）。
func (s *Service) familyExcept(ctx context.Context, familyID, exceptID uint64, typ, title, body, refType string, refID uint64) {
	if s == nil || s.f == nil {
		return
	}
	members, err := s.f.Members(ctx, familyID, true)
	if err != nil {
		log.Printf("WARN 取成员失败（通知跳过）family=%d: %v", familyID, err)
		return
	}
	for _, m := range members {
		if m.ID == exceptID {
			continue
		}
		s.one(ctx, familyID, m.ID, typ, title, body, refType, refID)
	}
}

// TaskSubmitted 有人提交了任务 → 通知其他成员去投票。
func (s *Service) TaskSubmitted(ctx context.Context, familyID, submitterID uint64, task *model.Task) {
	if task == nil {
		return
	}
	s.familyExcept(ctx, familyID, submitterID, TypeTaskSubmitted,
		"任务待投票", task.Title+" 已提交，等你投票", "task", task.ID)
}

// TaskDecided 任务投票出结果 → 通知接取者。
func (s *Service) TaskDecided(ctx context.Context, familyID, memberID uint64, taskID uint64, title, status string) {
	msg := "已通过，积分已到账"
	if status != "approved" {
		msg = "未通过，任务已关闭"
	}
	s.one(ctx, familyID, memberID, TypeTaskDecided, "任务「"+title+"」"+msg, "", "task", taskID)
}

// CouponPending 有人发起券核销 → 通知其他成员去投票。
func (s *Service) CouponPending(ctx context.Context, familyID, applicantID uint64, coupon *model.Coupon) {
	if coupon == nil {
		return
	}
	s.familyExcept(ctx, familyID, applicantID, TypeCouponPending,
		"核销待投票", coupon.ItemName+" 申请核销，等你投票", "coupon", coupon.ID)
}

// CouponDecided 券核销出结果 → 通知申请人。
func (s *Service) CouponDecided(ctx context.Context, familyID, memberID, couponID uint64, itemName, status string) {
	msg := "已核销"
	if status != "redeemed" {
		msg = "被驳回，券已退回可用"
	}
	s.one(ctx, familyID, memberID, TypeCouponDecided, "「"+itemName+"」"+msg, "", "coupon", couponID)
}

// TransferIn 收到赠送。
func (s *Service) TransferIn(ctx context.Context, familyID, toID uint64, amount int, fromName string) {
	s.one(ctx, familyID, toID, TypeTransferIn,
		"收到 "+fromName+" 的赠送", "+"+itoa(amount)+" 积分", "transfer", 0)
}

// Kicked 被移出家庭组。
func (s *Service) Kicked(ctx context.Context, familyID, memberID uint64, familyName string) {
	s.one(ctx, familyID, memberID, TypeKicked, "你已被移出「"+familyName+"」", "积分已清零，且不能再加入该家庭组", "family", familyID)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
