package clickhouseraft

import "testing"

func Test_getDDL(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		tableName string
		refSize   int
		want      string
	}{
		{
			name:      "test",
			tableName: "gg",
			refSize:   0,
			want:      ``,
		},
		{
			name:      "test",
			tableName: "gg1ref",
			refSize:   1,
			want:      ``,
		},
		{
			name:      "test",
			tableName: "gg2ref",
			refSize:   2,
			want:      ``,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDDL(tt.tableName, tt.refSize)
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("getDDL() = %v, want %v", got, tt.want)
			}
		})
	}
}
