package recontext

import (
	"context"
	"testing"
)


func TestBackground(t *testing.T) {
	tests := []struct{
		name string
		wantCtx context.Context
	}{
		{
			name: "Backgroundで作成したcontextと自前で同様に作成したcontextの比較",
			wantCtx: context.Background(),
		},
	}

	for _,tt := range tests {
		t.Run(tt.name,func(t *testing.T) {
			reCtx := Background()

			gotDeadline,gotOk := reCtx.Deadline()
			wantDeadline,wantOk := tt.wantCtx.Deadline()
			if gotDeadline != wantDeadline {
				t.Fatalf("deadline missmatch (-want %v, +got %v) \n",gotDeadline,wantDeadline)
			}

			if gotOk != wantOk {
				t.Fatalf("ok missmatch (-want %v, +got %v) \n",gotOk,wantOk)
			}
		})
	}
}