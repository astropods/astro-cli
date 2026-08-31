package evaldocument

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const evaluationRefPrefix = "agent/"

func evaluationRef(doc Document) (string, error) {
	canonical, err := canonicalJSON(doc)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return evaluationRefPrefix + hex.EncodeToString(sum[:]), nil
}

func canonicalJSON(doc Document) ([]byte, error) {
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal document: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("unmarshal document: %w", err)
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical document: %w", err)
	}
	return canonical, nil
}
