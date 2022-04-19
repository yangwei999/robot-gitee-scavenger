package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	sdk "github.com/opensourceways/go-gitee/gitee"
	"github.com/sirupsen/logrus"
)

const (
	prefixMergeRemind = "***PR Merge Reminder from Scavenger:*** \n"
	prefixClosureTips = "***PR Closure Tips from Scavenger:*** \n"
	closureTips       = `%s @%s This PR was closed for more than %d days of inactivity. You can recreate the PR if you need to merge the code.
该PR因闲置超过%d天而被关闭。如果需要合入代码，您可以重新创建PR。`
	mergeRemind = `%s This PR has been inactive for %d days (as no new commits, comments, configurations) and will be closed as it becomes obsolete after %d days of continued inactivity.
Please track the PR merging process in time, and respond according to the relevant prompts given by the robot to speed up the PR merging.
该PR已经闲置%d天（因没有新的提交、评论、配置）且将因继续闲置%d天后变得过时而被关闭。
请及时跟踪PR的合入流程并根据机器人给出的提示做出相应的响应以加速PR的合入。`
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
	bComments, cComments, err := prt.separateComments()
	if err != nil {
		log.Error(err)

		return
	}

	t, err := prt.prLastActiveTime(cComments)
	if err != nil {
		log.WithError(err).Errorf("%s/%s:%d  get lastActiveTime error", prt.org, prt.repo, prt.pr.Number)

		return
	}

	log.Infof("%s/%s:%d  lastActiveTime:%s", prt.org, prt.repo, prt.pr.Number, t)

	if prt.shouldClosed(t) {
		prt.handlePRClose(log)

		return
	}

	prt.handleRemindMerge(bComments, t, log)
}

func (prt *prTask) handlePRClose(log *logrus.Entry) {
	comment := fmt.Sprintf(closureTips, prefixClosureTips, prt.pr.User.Login, prt.maxOpenTime, prt.maxOpenTime)

	if err := prt.cli.CreatePRComment(prt.org, prt.repo, prt.pr.Number, comment); err != nil {
		log.Error(err)
	}

	if err := prt.cli.ClosePR(prt.org, prt.repo, prt.pr.Number); err != nil {
		log.Error(err)
	}
}

func (prt *prTask) shouldClosed(t string) bool {
	d := timeIntervalFromNow(t)

	return d > prt.maxOpenTime
}

func (prt *prTask) handleRemindMerge(comments []sdk.PullRequestComments, t string, log *logrus.Entry) {
	d := timeIntervalFromNow(t)
	if d < prt.interval {
		return
	}

	rs, find := prt.findRecentMergeReminder(comments)
	if !find {
		prt.addMergeRemindComment(d, t, log)

		return
	}

	if d := timeIntervalFromNow(rs.UpdatedAt); d > prt.interval {
		prt.addMergeRemindComment(d, t, log)
	}
}

func (prt *prTask) addMergeRemindComment(inactiveTime int, lastActiveTime string, log *logrus.Entry) {
	d := prt.maxOpenTime - timeIntervalFromNow(lastActiveTime)
	comment := fmt.Sprintf(mergeRemind, prefixMergeRemind, inactiveTime, d, inactiveTime, d)

	if err := prt.cli.CreatePRComment(prt.org, prt.repo, prt.pr.Number, comment); err != nil {
		log.Error(err)
	}
}

func (prt *prTask) findRecentMergeReminder(comments []sdk.PullRequestComments) (c sdk.PullRequestComments, find bool) {
	if len(comments) == 0 {
		return
	}

	for _, v := range comments {
		if strings.HasPrefix(v.Body, prefixMergeRemind) {
			return v, true
		}
	}

	return
}

func (prt *prTask) prLastActiveTime(comments []sdk.PullRequestComments) (string, error) {
	lastActiveTime := prt.pr.CreatedAt

	if len(comments) > 0 && t1BeforeT2(lastActiveTime, comments[0].UpdatedAt) {
		lastActiveTime = comments[0].UpdatedAt
	}

	t, err := prt.lastOperateTime()
	if err != nil {
		return lastActiveTime, err
	}

	if t != "" && t1BeforeT2(lastActiveTime, t) {
		lastActiveTime = t
	}

	t, err = prt.lastCommitTime()
	if err != nil {
		return lastActiveTime, err
	}

	if t != "" && t1BeforeT2(lastActiveTime, t) {
		lastActiveTime = t
	}

	return lastActiveTime, nil
}

// separateComments Separate the comments of the robot and the comments of ordinary users
// and sort them in reverse order of update time.
func (prt *prTask) separateComments() (botComments, commComments []sdk.PullRequestComments, err error) {
	comments, err := prt.cli.ListPRComments(prt.org, prt.repo, prt.pr.Number)
	if err != nil {
		return
	}

	var bcs, ccs []sdk.PullRequestComments

	for _, v := range comments {
		if v.User.Login == prt.getBotName() {
			bcs = append(bcs, v)
		} else {
			ccs = append(ccs, v)
		}
	}

	if len(bcs) > 1 {
		sort.SliceStable(bcs, func(i, j int) bool {
			return !t1BeforeT2(bcs[i].UpdatedAt, bcs[j].UpdatedAt)
		})
	}

	if len(ccs) > 1 {
		sort.SliceStable(ccs, func(i, j int) bool {
			return !t1BeforeT2(ccs[i].UpdatedAt, ccs[j].UpdatedAt)
		})
	}

	return bcs, ccs, nil
}

func (prt *prTask) lastOperateTime() (string, error) {
	logs, err := prt.cli.ListPROperationLogs(prt.org, prt.repo, prt.pr.Number)
	if err != nil {
		return "", err
	}

	lastTime := ""

	for _, v := range logs {
		if t := v.CreatedAt; lastTime == "" || t1BeforeT2(lastTime, t) {
			lastTime = t
		}
	}

	return lastTime, nil
}

func (prt *prTask) lastCommitTime() (string, error) {
	commits, err := prt.cli.GetPRCommits(prt.org, prt.repo, prt.pr.Number)
	if err != nil {
		return "", err
	}

	lastCommitTime := ""

	for _, v := range commits {
		t := v.Commit.GetCommitter().GetDate().Format(time.RFC3339)
		if lastCommitTime == "" || t1BeforeT2(lastCommitTime, t) {
			lastCommitTime = t
		}
	}

	return lastCommitTime, nil
}

func t1BeforeT2(t1, t2 string) bool {
	tt1, err1 := time.Parse(time.RFC3339, t1)
	tt2, err2 := time.Parse(time.RFC3339, t2)

	return err1 == nil && err2 == nil && tt1.Before(tt2)
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
