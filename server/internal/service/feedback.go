// Package service：用户反馈（2026-11-07）。
package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"familypoints/server/internal/model"
	"familypoints/server/internal/repo"
)

// FeedbackService 反馈提交与后台查看。
type FeedbackService struct {
	feedbacks *repo.Feedback
}

// NewFeedback 构造。
func NewFeedback(f *repo.Feedback) *FeedbackService { return &FeedbackService{feedbacks: f} }

// ErrFeedbackTooLong 反馈超长。
var ErrFeedbackTooLong = fmt.Errorf("%w：反馈内容最多 500 字", ErrInvalidInput)

// Submit 成员提交反馈（登录态，无需入组）。
func (s *FeedbackService) Submit(ctx context.Context, me *model.Member, content string) error {
	content = strings.TrimSpace(content)
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("%w：反馈内容不能为空", ErrInvalidInput)
	}
	if utf8.RuneCountInString(content) > 500 {
		return ErrFeedbackTooLong
	}
	return s.feedbacks.Create(ctx, me.ID, me.Username, content)
}

// List 后台列表。
func (s *FeedbackService) List(ctx context.Context, handled int, limit int) ([]model.Feedback, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.feedbacks.List(ctx, handled, limit)
}

// Handle 标记已处理。
func (s *FeedbackService) Handle(ctx context.Context, id uint64) error {
	return s.feedbacks.MarkHandled(ctx, id)
}
