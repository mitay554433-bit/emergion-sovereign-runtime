package main

import (
	"os"
	"strconv"
	"strings"

	"emergion-sovereign-runtime/pkg/fieldapi"
)

// BatchParams is the POST /decide-batch-human body.
type BatchParams struct {
	EmergIONIDs  []string `json:"emergion_ids"`
	Reason       string   `json:"reason"`
	ConfirmCount int      `json:"confirm_count"`
}

// batchCap is the server-side batch limit (flag -batch-cap, env FIELDD_BATCH_CAP).
var batchCap = fieldapi.DefaultBatchCap

func batchCapFromEnv() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("FIELDD_BATCH_CAP"))); err == nil && n > 0 {
		return n
	}
	return fieldapi.DefaultBatchCap
}
