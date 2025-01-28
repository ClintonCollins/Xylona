package xycrypt

import (
	"testing"
)

func TestCompareHashAndString(t *testing.T) {
	testHash1, errGenerate := GenerateHashFromString("SomeTestString", DefaultHashParameters)
	if errGenerate != nil {
		t.Errorf("Error generating hash: %v", errGenerate)
		return
	}
	testHash2, errGenerate := GenerateHashFromString("77%F!^(f@4babl0*3&7v8o7@lpy^2)#3^Nz+^oIgzp8)73Qltscw6nNG5QE$z05z", DefaultHashParameters)
	if errGenerate != nil {
		t.Errorf("Error generating hash: %v", errGenerate)
		return
	}
	type args struct {
		encodedHash []byte
		input       string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "match 1",
			args: args{
				encodedHash: testHash1,
				input:       "SomeTestString",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "match 2",
			args: args{
				encodedHash: testHash2,
				input:       "77%F!^(f@4babl0*3&7v8o7@lpy^2)#3^Nz+^oIgzp8)73Qltscw6nNG5QE$z05z",
			},
			want:    true,
			wantErr: false,
		},

		{
			name: "not match 1",
			args: args{
				encodedHash: testHash1,
				input:       "swercv#!@#sdfsdz",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "not match 2",
			args: args{
				encodedHash: testHash1,
				input:       "someteststring",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "invalid version",
			args: args{
				encodedHash: []byte("$argon2id$17$98304$4$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid algorithm",
			args: args{
				encodedHash: []byte("$bcrypt$17$98304$4$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid format",
			args: args{
				encodedHash: []byte("$bcrypt$17$98304$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid cost",
			args: args{
				encodedHash: []byte("$bcrypt$17$98304$3$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},


	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareHashAndString(tt.args.encodedHash, tt.args.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareHashAndString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CompareHashAndString() got = %v, want %v", got, tt.want)
			}
		})
	}
}
