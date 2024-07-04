package inmemory

import (
	"reflect"
	"testing"
)

type TestType struct {
	Name string
	Role []string
}

func Test_copyData_TestType(t *testing.T) {
	type args struct {
		a TestType
	}
	tests := []struct {
		name string
		args args
		want TestType
	}{
		{
			name: "Test decode encode",
			args: args{
				a: TestType{
					Name: "HELLO",
					Role: []string{"WORLD", "HAI"},
				},
			},
			want: TestType{
				Name: "HELLO",
				Role: []string{"WORLD", "HAI"},
			},
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
