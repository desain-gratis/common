package clickhouseraft

type state struct {
	EventIndexes map[string]*uint64 `json:"event_indexes"`
}
