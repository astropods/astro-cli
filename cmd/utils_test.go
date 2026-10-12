package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWriteTableAlignsColumnsAndLeavesTheLastUnpadded(t *testing.T) {
	buf := &bytes.Buffer{}

	writeTable(buf, []string{"Name", "Blueprint", "Deployment"}, [][]string{
		{"eval-a", "support-bot", "dep1"},
		{"é…", "x", "dep22"},
	})

	assert.Equal(t, strings.Join([]string{
		"Name    Blueprint    Deployment",
		"eval-a  support-bot  dep1",
		"é…      x            dep22",
	}, "\n")+"\n", buf.String(), "cells pad to the widest in each column, counted in runes")
}
