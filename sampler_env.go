// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry // import "go.opentelemetry.io/otel/sdk/trace"

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/sdk/trace"
)

const (
	tracesSamplerKey    = "OTEL_TRACES_SAMPLER"
	tracesSamplerArgKey = "OTEL_TRACES_SAMPLER_ARG"

	samplerAlwaysOn                = "always_on"
	samplerAlwaysOff               = "always_off"
	samplerTraceIDRatio            = "traceidratio"
	samplerParentBasedAlwaysOn     = "parentbased_always_on"
	samplerParsedBasedAlwaysOff    = "parentbased_always_off"
	samplerParentBasedTraceIDRatio = "parentbased_traceidratio"
)

type errUnsupportedSampler string

func (e errUnsupportedSampler) Error() string {
	return fmt.Sprintf("unsupported sampler: %s", string(e))
}

var (
	errNegativeTraceIDRatio       = errors.New("invalid trace ID ratio: less than 0.0")
	errGreaterThanOneTraceIDRatio = errors.New("invalid trace ID ratio: greater than 1.0")
)

type samplerArgParseError struct {
	parseErr error
}

func (e samplerArgParseError) Error() string {
	return fmt.Sprintf("parsing sampler argument: %s", e.parseErr.Error())
}

func (e samplerArgParseError) Unwrap() error {
	return e.parseErr
}

func samplerFromEnv() (trace.Sampler, error) {
	sampler, ok := os.LookupEnv(tracesSamplerKey)
	if !ok {
		return nil, nil
	}

	sampler = strings.ToLower(strings.TrimSpace(sampler))
	samplerArg, hasSamplerArg := os.LookupEnv(tracesSamplerArgKey)
	samplerArg = strings.TrimSpace(samplerArg)

	switch sampler {
	case samplerAlwaysOn:
		return trace.AlwaysSample(), nil
	case samplerAlwaysOff:
		return trace.NeverSample(), nil
	case samplerTraceIDRatio:
		if !hasSamplerArg {
			return trace.TraceIDRatioBased(1.0), nil
		}
		return parseTraceIDRatio(samplerArg)
	case samplerParentBasedAlwaysOn:
		return trace.ParentBased(trace.AlwaysSample()), nil
	case samplerParsedBasedAlwaysOff:
		return trace.ParentBased(trace.NeverSample()), nil
	case samplerParentBasedTraceIDRatio:
		if !hasSamplerArg {
			return trace.ParentBased(trace.TraceIDRatioBased(1.0)), nil
		}
		ratio, err := parseTraceIDRatio(samplerArg)
		return trace.ParentBased(ratio), err
	default:
		return nil, errUnsupportedSampler(sampler)
	}
}

func parseTraceIDRatio(arg string) (trace.Sampler, error) {
	v, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		return trace.TraceIDRatioBased(1.0), samplerArgParseError{err}
	}
	if v < 0.0 {
		return trace.TraceIDRatioBased(1.0), errNegativeTraceIDRatio
	}
	if v > 1.0 {
		return trace.TraceIDRatioBased(1.0), errGreaterThanOneTraceIDRatio
	}

	return trace.TraceIDRatioBased(v), nil
}
