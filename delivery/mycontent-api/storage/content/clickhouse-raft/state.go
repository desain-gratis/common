package clickhouseraft

type state struct {
	EventIndexes   map[string]*uint64 `json:"event_indexes"`
	VersionIndexes map[string]*uint64 `json:"version_indexes"`
}
