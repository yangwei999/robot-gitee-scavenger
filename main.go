package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/opensourceways/community-robot-lib/config"
	"github.com/opensourceways/community-robot-lib/giteeclient"
	"github.com/opensourceways/community-robot-lib/logrusutil"
	liboptions "github.com/opensourceways/community-robot-lib/options"
	"github.com/opensourceways/community-robot-lib/secret"
	"github.com/panjf2000/ants/v2"
	"github.com/sirupsen/logrus"
)

type options struct {
	gitee      liboptions.GiteeOptions
	configFile string
}

func (o *options) Validate() error {

	return o.gitee.Validate()
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options

	o.gitee.AddFlags(fs)
	fs.StringVar(&o.configFile, "config-file", "", "Path to config file.")

	_ = fs.Parse(args)

	return o
}

func main() {
	logrusutil.ComponentInit(botName)

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	cfg, err := getConfig(o.configFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting config.")
	}

	c, err := genClient(o.gitee.TokenPath)
	if err != nil {
		logrus.WithError(err).Fatal("Error generating client.")
	}

	pool, err := newPool(cfg.ConcurrentSize, logrus.WithField("pool", "logger"))
	if err != nil {
		logrus.WithError(err).Fatal("Error starting goroutine pool.")
	}

	defer pool.Release()

	v, err := c.GetBot()
	if err != nil {
		logrus.WithError(err).Error("Error get bot name")
	}

	bot := newRobot(c, pool, &cfg, v.Login)

	run(bot)
}

func run(bot *robot) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wg.Add(1)

	go func(ctx context.Context) {
		defer wg.Done()

		select {
		case <-ctx.Done():
			logrus.Info("receive done. exit normally")

			return
		case <-sig:
			logrus.Info("receive exit signal")
			cancel()

			return
		}
	}(ctx)

	log := logrus.WithField("action", "run")

	bot.run(ctx, log)
}

func getConfig(configFile string) (botConfig, error) {
	agent := config.NewConfigAgent(func() config.Config {
		return &configuration{}
	})

	if err := agent.Start(configFile); err != nil {
		return botConfig{}, err
	}

	agent.Stop()

	_, v := agent.GetConfig()

	if cfg, ok := v.(*configuration); ok {
		return cfg.Config, nil
	}

	return botConfig{}, fmt.Errorf("can't convert the configuration")
}

func genClient(tokenPath string) (iClient, error) {
	secretAgent := new(secret.Agent)

	if err := secretAgent.Start([]string{tokenPath}); err != nil {
		return nil, err
	}

	secretAgent.Stop()

	t := secretAgent.GetTokenGenerator(tokenPath)

	return giteeclient.NewClient(t), nil
}

func newPool(size int, log ants.Logger) (*ants.Pool, error) {
	return ants.NewPool(size, ants.WithOptions(ants.Options{
		Logger: log,
	}))
}
