package dnsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type SeedData struct {
	Zones   []Zone   `json:"zones"`
	Records []Record `json:"records"`
}

func LoadSeedData(path string) (SeedData, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SeedData{}, fmt.Errorf("read seed data: %w", err)
	}

	var seed SeedData
	if err := json.Unmarshal(content, &seed); err != nil {
		return SeedData{}, fmt.Errorf("decode seed data: %w", err)
	}

	return seed, nil
}

func ApplySeedData(ctx context.Context, repo Repository, seed SeedData) error {
	for _, zone := range seed.Zones {
		if err := repo.SaveZone(ctx, zone); err != nil {
			return err
		}
	}

	for _, record := range seed.Records {
		if err := repo.SaveRecord(ctx, record); err != nil {
			return err
		}
	}

	return nil
}
