package splunk

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const configDirName = "splsearch"

type Store interface {
	Get(target Target) (*AuthRecord, error)
	List() ([]AuthRecord, error)
	Set(target Target, record AuthRecord) error
	Delete(target Target) (bool, error)
}

type FileStore struct {
	configDir string
}

type storeData struct {
	Records map[string]AuthRecord `json:"records"`
}

func DefaultConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(".", ".config", configDirName)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, configDirName)
}

func NewFileStore(configDir string) *FileStore {
	return &FileStore{configDir: configDir}
}

func (s *FileStore) Get(target Target) (*AuthRecord, error) {
	data, err := s.read()
	if err != nil {
		return nil, err
	}
	if record, ok := data.Records[target.Key]; ok {
		return &record, nil
	}
	for _, record := range data.Records {
		if containsString(record.Aliases, target.Key) {
			copyRecord := record
			return &copyRecord, nil
		}
	}
	return nil, nil
}

func (s *FileStore) List() ([]AuthRecord, error) {
	data, err := s.read()
	if err != nil {
		return nil, err
	}
	records := make([]AuthRecord, 0, len(data.Records))
	for _, record := range data.Records {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].URL < records[j].URL
	})
	return records, nil
}

func (s *FileStore) Set(target Target, record AuthRecord) error {
	if record.Method == MethodSessionKey || record.SessionKey != "" {
		return fmt.Errorf("ephemeral session authentication cannot be persisted")
	}
	data, err := s.read()
	if err != nil {
		return err
	}
	if data.Records == nil {
		data.Records = map[string]AuthRecord{}
	}
	record.URL = target.Key
	record.Aliases = uniqueStrings(append(record.Aliases, target.Bases...)...)
	for key, existing := range data.Records {
		if key == target.Key || containsString(record.Aliases, key) || stringSlicesOverlap(existing.Aliases, record.Aliases) {
			delete(data.Records, key)
		}
	}
	data.Records[target.Key] = record
	return s.write(data)
}

func (s *FileStore) Delete(target Target) (bool, error) {
	data, err := s.read()
	if err != nil {
		return false, err
	}
	removed := false
	if _, ok := data.Records[target.Key]; ok {
		delete(data.Records, target.Key)
		removed = true
	}
	for key, record := range data.Records {
		if containsString(record.Aliases, target.Key) {
			delete(data.Records, key)
			removed = true
		}
	}
	if !removed {
		return false, nil
	}
	return true, s.write(data)
}

func (s *FileStore) path() string {
	return filepath.Join(s.configDir, "auth.json")
}

func (s *FileStore) read() (storeData, error) {
	path := s.path()
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return storeData{Records: map[string]AuthRecord{}}, nil
	}
	if err != nil {
		return storeData{}, fmt.Errorf("read auth store: %w", err)
	}
	if len(raw) == 0 {
		return storeData{Records: map[string]AuthRecord{}}, nil
	}
	var data storeData
	if err := json.Unmarshal(raw, &data); err != nil {
		return storeData{}, fmt.Errorf("parse auth store: %w", err)
	}
	if data.Records == nil {
		data.Records = map[string]AuthRecord{}
	}
	return data, nil
}

func (s *FileStore) write(data storeData) error {
	if err := os.MkdirAll(s.configDir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode auth store: %w", err)
	}
	path := s.path()
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("write auth store: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func stringSlicesOverlap(left, right []string) bool {
	for _, value := range left {
		if containsString(right, value) {
			return true
		}
	}
	return false
}
