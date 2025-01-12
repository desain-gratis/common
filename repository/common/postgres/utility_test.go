package postgres

import (
	"reflect"
	"testing"

	m "github.com/desain-gratis/common/repository"
)

func TestParse(t *testing.T) {
	type args struct {
		in []byte
	}
	tests := []struct {
		name    string
		args    args
		want    m.AuthorizedUser
		wantErr bool
	}{
		{
			name: "Test serde",
			args: args{
				in: []byte(`{"id": "hello", "default_homepage": "hai"}`),
			},
			want: m.AuthorizedUser{
				ID:              "hello",
				DefaultHomepage: "hai",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse[m.AuthorizedUser](tt.args.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}
