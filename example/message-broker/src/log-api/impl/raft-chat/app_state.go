package raftchat

type state struct {
	ChatIndex *uint64 `json:"chat_index"`
}
