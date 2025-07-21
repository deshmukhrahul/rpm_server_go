package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type RepoConfig struct {
	ID     string   `yaml:"id"`
	Paths  []string `yaml:"paths"`
	TagDir string   `yaml:"tag_dir,omitempty"`
}

type AppConfig struct {
	BasePath string       `yaml:"base_path"`
	TagsBase string       `yaml:"tags_base"`
	Repos    []RepoConfig `yaml:"repos"`
}

func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", path, err)
	}

	var config AppConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if config.TagsBase == "" {
		config.TagsBase = "repo/tags"
	}

	return &config, nil
}

func (c *AppConfig) GetRepoPaths(repoID string) ([]string, error) {
	for _, repo := range c.Repos {
		if repo.ID == repoID {
			resolvedPaths := make([]string, len(repo.Paths))
			for i, p := range repo.Paths {
				resolvedPaths[i] = filepath.Join(c.BasePath, p)
			}
			return resolvedPaths, nil
		}
	}
	return nil, fmt.Errorf("repo '%s' not found in config file", repoID)
}

func (c *AppConfig) GetTagsPathForRepo(repoID string) string {
	for _, repo := range c.Repos {
		if repo.ID == repoID && repo.TagDir != "" {
			return repo.TagDir
		}
	}
	return c.TagsBase
}
