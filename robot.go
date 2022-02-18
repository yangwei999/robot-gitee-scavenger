package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/opensourceways/community-robot-lib/config"
	"github.com/opensourceways/community-robot-lib/giteeclient"
	sdk "github.com/opensourceways/go-gitee/gitee"
	"github.com/panjf2000/ants/v2"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const botName = "scavenger"

type iClient interface {
	GetBot() (sdk.User, error)
	GetRepos(org string) ([]sdk.Project, error)
	GetPullRequests(org, repo string, opts giteeclient.ListPullRequestOpt) ([]sdk.PullRequest, error)
	ClosePR(org, repo string, number int32) error
	CreatePRComment(org, repo string, number int32, comment string) error
	ListPRComments(org, repo string, number int32) ([]sdk.PullRequestComments, error)
	GetPRCommits(org, repo string, number int32) ([]sdk.PullRequestCommits, error)
	ListPROperationLogs(org, repo string, number int32) ([]sdk.OperateLog, error)
}

func newRobot(cli iClient, p *ants.Pool, cfg *botConfig, botName string) *robot {
	return &robot{cli: cli, pool: p, cfg: cfg, botName: botName}
}

type robot struct {
	cli     iClient
	pool    *ants.Pool
	cfg     *botConfig
	wg      sync.WaitGroup
	botName string
}

func (bot *robot) NewConfig() config.Config {
	return &configuration{}
}

func (bot *robot) run(ctx context.Context, log *logrus.Entry) {
	th := runtime.NumCPU()
	if th < len(bot.cfg.Repos) {
		th = len(bot.cfg.Repos)
	}

	ch := make(chan string, th)

	bot.filterRepos(ctx, ch, log)
	bot.processRepos(ctx, ch, log)

	bot.wg.Wait()
}

func (bot *robot) filterRepos(ctx context.Context, in chan<- string, log *logrus.Entry) {
	bot.wg.Add(1)

	go func() {
		defer bot.wg.Done()

		cache := sets.NewString()
		eCache := sets.NewString(bot.cfg.ExcludedRepos...)

		validSend := func(r string) {
			if !cache.Has(r) && !eCache.Has(r) {
				cache.Insert(r)
				in <- r
			}
		}

		for _, v := range bot.cfg.Repos {
			if isCancelled(ctx) {
				break
			}

			if strings.Contains(v, "/") {
				validSend(v)

				continue
			}

			log.Infof("load %s all repos ", v)

			rps, err := bot.cli.GetRepos(v)
			if err != nil {
				log.Error(err)
			}

			for _, r := range rps {
				validSend(r.GetFullName())
			}
		}

		close(in)
	}()
}

func (bot *robot) processRepos(ctx context.Context, out <-chan string, log *logrus.Entry) {
	threshold := cap(out)
	if threshold == 0 {
		threshold = 1
	}

	for i := 0; i < threshold; i++ {
		bot.wg.Add(1)

		go func() {
			defer bot.wg.Done()

			for v := range out {
				if isCancelled(ctx) {
					continue
				}

				if err := bot.genTask(ctx, v, log); err != nil {
					log.Errorf("gen task for %s occur error: %s", v, err.Error())
				}
			}
		}()
	}
}

func (bot *robot) genTask(ctx context.Context, repo string, log *logrus.Entry) error {
	spl := strings.Split(repo, "/")

	if validLength := 2; len(spl) != validLength {
		return fmt.Errorf("%s is invalid full name of repository", repo)
	}

	org, repo := spl[0], spl[1]

	prs, err := bot.cli.GetPullRequests(org, repo, giteeclient.ListPullRequestOpt{State: "open"})
	if err != nil {
		return err
	}

	for _, v := range prs {
		if isCancelled(ctx) {
			break
		}

		pt := &prTask{
			cli:         bot.cli,
			ctx:         ctx,
			org:         org,
			repo:        repo,
			pr:          v,
			interval:    bot.cfg.MergeRemindIntervals,
			maxOpenTime: bot.cfg.MaximumOpenTime,
			getBotName:  bot.getBotName,
		}

		if err := bot.submitTask(pt, log); err != nil {
			log.Error(err)
		}
	}

	return nil
}

func (bot *robot) submitTask(t tasker, log *logrus.Entry) error {
	bot.wg.Add(1)

	return bot.pool.Submit(func() {
		defer bot.wg.Done()
		t.exec(log)
	})
}

func (bot *robot) getBotName() string {
	return bot.botName
}

func isCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
