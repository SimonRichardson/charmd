// Copyright (c) 2023 Canonical Ltd
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License version 3 as
// published by the Free Software Foundation.

package client_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/canonical/tc"

	"github.com/SimonRichardson/charmd/client"
)

func Test(t *testing.T) { tc.InternalTestingT(t) }

// clientSuite retains the request and response parts of Charmd's test fixture.
type clientSuite struct {
	cli     *client.Client
	req     *http.Request
	rsp     string
	err     error
	restore func()
}

var _ = tc.InternalSuite(&clientSuite{})

func (cs *clientSuite) SetUpTest(c *tc.C) {
	var err error
	cs.cli, err = client.New(nil)
	c.Assert(err, tc.IsNil)
	cs.cli.SetDoer(cs)
	cs.err = nil
	cs.req = nil
	cs.rsp = ""
	cs.restore = client.FakeDoRetry(time.Millisecond, 10*time.Millisecond)
}

func (cs *clientSuite) TearDownTest(c *tc.C) { cs.restore() }

func (cs *clientSuite) Do(req *http.Request) (*http.Response, error) {
	cs.req = req
	return &http.Response{Body: io.NopCloser(strings.NewReader(cs.rsp)), StatusCode: 200}, cs.err
}
