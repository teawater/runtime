// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package utils

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

type sourceHook struct{}

func (hook *sourceHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (hook *sourceHook) Fire(entry *logrus.Entry) error {
	var (
		file     string
		function string
		line     int
		got      bool = false
	)

	for i := 0; i < 10; i++ {
		var (
			pc uintptr
			ok bool
		)
		pc, file, line, ok = runtime.Caller(5 + i)
		if ok && pc != 0 && !strings.Contains(file, "/sirupsen/logrus/") {
			frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
			function = frame.Function
			if !strings.Contains(function, "Logger") {
				got = true
				break
			}
		}
	}
	if got {
		entry.Data["source"] = fmt.Sprintf("%s:%d", file, line)
	}
	return nil
}

type sourceKey struct{}

func WithSource(ctx context.Context) context.Context {
	return context.WithValue(ctx, sourceKey{}, true)
}

func AddSource(ctx context.Context, logger *logrus.Entry) {
	if ctx.Value(sourceKey{}) != nil {
		logger.Logger.AddHook(&sourceHook{})
	}
}
