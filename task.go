package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opensourceways/community-robot-lib/giteeclient"
	sdk "github.com/opensourceways/go-gitee/gitee"
	"github.com/sirupsen/logrus"
)

const (
	prefixMergeRemind = "***PR Merge Reminder from Scavenger Bot:*** \n"
	prefixClosureTips = "***PR Closure Tips from Scavenger Bot:*** \n"
	closureTips       = "%s @%s PR closed for not being merged for more than %d days."
	mergeRemind       = `%s PRs will be obsolete and closed after %d days.
Please track the PR merging process in time, and respond according to the relevant prompts given by the robot to speed up the PR merging.`
)

type tasker interface {
	exec(log *logrus.Entry)
}

type prTask struct {
	cli        iClient
	ctx        context.Context
	getBotName func() string

	org  string
	repo string
	pr   sdk.PullRequest

	interval    int
	maxOpenTime int
}

func (prt *prTask) exec(log *logrus.Entry) {
	log.Infof("%s/%s:%d  createTime:%s", prt.org, prt.repo, prt.pr.Number, prt.pr.CreatedAt)

	if prt.shouldClosed() {
		prt.handlePRClose(log)

		return
	}

	prt.handleRemindMerge(log)
}

func (prt *prTask) handlePRClose(log *logrus.Entry) {
	comment := fmt.Sprintf(closureTips, prefixClosureTips, prt.pr.User.Login, prt.maxOpenTime)

	if err := prt.cli.CreatePRComment(prt.org, prt.repo, prt.pr.Number, comment); err != nil {
		log.Error(err)
	}

	if err := prt.cli.ClosePR(prt.org, prt.repo, prt.pr.Number); err != nil {
		log.Error(err)
	}
}

func (prt *prTask) shouldClosed() bool {
	d := timeIntervalFromNow(prt.pr.CreatedAt)

	return d > prt.maxOpenTime
}

func (prt *prTask) handleRemindMerge(log *logrus.Entry) {
	d := timeIntervalFromNow(prt.pr.CreatedAt)
	if d < prt.interval {
		return
	}

	rs, find := prt.findRecentMergeReminder(log)
	if !find {
		prt.addMergeRemindComment(log)

		return
	}

	if d := timeIntervalFromNow(rs.CreatedAt.Format(time.RFC3339)); d > prt.interval {
		prt.addMergeRemindComment(log)
	}
}

func (prt *prTask) addMergeRemindComment(log *logrus.Entry) {
	d := prt.maxOpenTime - timeIntervalFromNow(prt.pr.CreatedAt)
	comment := fmt.Sprintf(mergeRemind, prefixMergeRemind, d)

	if err := prt.cli.CreatePRComment(prt.org, prt.repo, prt.pr.Number, comment); err != nil {
		log.Error(err)
	}
}

func (prt *prTask) findRecentMergeReminder(log *logrus.Entry) (c giteeclient.BotComment, find bool) {
	comments, err := prt.cli.ListPRComments(prt.org, prt.repo, prt.pr.Number)
	if err != nil {
		log.Error(err)

		return
	}

	mergeReminds := giteeclient.FindBotComment(comments, prt.getBotName(), func(s string) bool {
		return strings.HasPrefix(s, prefixMergeRemind)
	})

	if len(mergeReminds) == 0 {
		return
	}

	giteeclient.SortBotComments(mergeReminds)

	return mergeReminds[len(mergeReminds)-1], true
}

func timeIntervalFromNow(t string) int {
	tl, err := time.Parse(time.RFC3339, t)
	if err != nil {
		return 0
	}

	t2 := time.Now()
	dayHours := 24

	return int(t2.Sub(tl).Hours() / float64(dayHours))
}
