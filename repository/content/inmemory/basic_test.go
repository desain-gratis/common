package inmemory

import (
	"reflect"
	"testing"
)

func Test_copyData_TestType(t *testing.T) {
	type args struct {
		a []byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "Test decode encode",
			args: args{
				a: []byte(`{"name" : "hello", "role": ["world", "hai"]}`),
			},
			want: []byte(`{"name" : "hello", "role": ["world", "hai"]}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := copyData(tt.args.a); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("copyData() = %v, want %v", got, tt.want)
			}
		})
	}
}
