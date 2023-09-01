package password

import (
	"testing"
)

func TestPolicies_Validate(t *testing.T) {
	type fields struct {
		minCharacters          int
		minLowerCaseCharacters int
		minUpperCaseCharacters int
		minDigits              int
		minSpecialCharacters   int
		specialCharacters      string
	}
	tests := []struct {
		name    string
		fields  fields
		args    string
		wantErr bool
	}{
		{
			name: "all in one",
			fields: fields{
				minCharacters:          100,
				minLowerCaseCharacters: 29,
				minUpperCaseCharacters: 29,
				minDigits:              10,
				minSpecialCharacters:   32,
				specialCharacters:      "",
			},
			args: "1234567890abcdefghijklmnopqrstuvwxyzäöüABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÜ!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~",
		},
		{
			name: "exactly one",
			fields: fields{
				minCharacters:          1,
				minLowerCaseCharacters: 1,
				minUpperCaseCharacters: 1,
				minDigits:              1,
				minSpecialCharacters:   1,
				specialCharacters:      "-",
			},
			args: "0äÖ-",
		},
		{
			name: "exactly + special",
			fields: fields{
				minCharacters:          12,
				minLowerCaseCharacters: 3,
				minUpperCaseCharacters: 3,
				minDigits:              1,
				minSpecialCharacters:   9,
				specialCharacters:      "-世界 ßAaBb",
			},
			args: "0äÖ-世界 ßAaBb",
		},
		{
			name: "exactly cyrillic",
			fields: fields{
				minCharacters:          6,
				minLowerCaseCharacters: 3,
				minUpperCaseCharacters: 3,
				minDigits:              0,
				minSpecialCharacters:   0,
				specialCharacters:      "",
			},
			args: "іІїЇЯяЙй",
		},
		{
			name: "error",
			fields: fields{
				minCharacters:          2,
				minLowerCaseCharacters: 2,
				minUpperCaseCharacters: 2,
				minDigits:              2,
				minSpecialCharacters:   2,
				specialCharacters:      "",
			},
			args:    "0äÖ-",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewPasswordPolicies(
				tt.fields.minCharacters,
				tt.fields.minLowerCaseCharacters,
				tt.fields.minUpperCaseCharacters,
				tt.fields.minDigits,
				tt.fields.minSpecialCharacters,
				tt.fields.specialCharacters,
			)
			if err != nil {
				t.Error(err)
			}
			if err := s.Validate(tt.args); (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestPasswordPolicies_Count(t *testing.T) {
	type want struct {
		wantCharacters          int
		wantLowerCaseCharacters int
		wantUpperCaseCharacters int
		wantDigits              int
		wantSpecialCharacters   int
		specialCharacters       string
	}
	tests := []struct {
		name    string
		fields  want
		args    string
		wantErr bool
	}{
		{
			name: "all in one",
			fields: want{
				wantCharacters:          100,
				wantLowerCaseCharacters: 29,
				wantUpperCaseCharacters: 29,
				wantDigits:              10,
				wantSpecialCharacters:   32,
			},
			args: "1234567890abcdefghijklmnopqrstuvwxyzäöüABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÜ!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~",
		},
		{
			name: "length only",
			fields: want{
				wantCharacters:          4,
				wantLowerCaseCharacters: 0,
				wantUpperCaseCharacters: 0,
				wantDigits:              0,
				wantSpecialCharacters:   0,
			},
			args: "世界 ß",
		},
		{
			name: "length only",
			fields: want{
				wantCharacters:          8,
				wantLowerCaseCharacters: 2,
				wantUpperCaseCharacters: 2,
				wantDigits:              0,
				wantSpecialCharacters:   8,
				specialCharacters:       "世界 ßAaBb",
			},
			args: "世界 ßAaBb",
		},
		{
			name: "empty",
			fields: want{
				wantCharacters:          0,
				wantLowerCaseCharacters: 0,
				wantUpperCaseCharacters: 0,
				wantDigits:              0,
				wantSpecialCharacters:   0,
			},
			args: "",
		},
		{
			name: "check '-' sing",
			fields: want{
				wantCharacters:          33,
				wantLowerCaseCharacters: 0,
				wantUpperCaseCharacters: 0,
				wantDigits:              0,
				wantSpecialCharacters:   5,
				specialCharacters:       `_!-+`,
			},
			args: "!!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i, err := NewPasswordPolicies(
				tt.fields.wantCharacters,
				tt.fields.wantLowerCaseCharacters,
				tt.fields.wantUpperCaseCharacters,
				tt.fields.wantDigits,
				tt.fields.wantSpecialCharacters,
				tt.fields.specialCharacters,
			)
			if err != nil {
				t.Error(err)
				return
			}
			s := i.(*Policies)
			if got := s.count(tt.args); got != tt.fields.wantCharacters {
				t.Errorf("count() = %v, want %v", got, tt.fields.wantCharacters)
			}
			if got := s.countLowerCaseCharacters(tt.args); got != tt.fields.wantLowerCaseCharacters {
				t.Errorf("countLowerCaseCharacters() = %v, want %v", got, tt.fields.wantLowerCaseCharacters)
			}
			if got := s.countUpperCaseCharacters(tt.args); got != tt.fields.wantUpperCaseCharacters {
				t.Errorf("countUpperCaseCharacters() = %v, want %v", got, tt.fields.wantUpperCaseCharacters)
			}
			if got := s.countDigits(tt.args); got != tt.fields.wantDigits {
				t.Errorf("countDigits() = %v, want %v", got, tt.fields.wantDigits)
			}
			if got := s.countSpecialCharacters(tt.args); got != tt.fields.wantSpecialCharacters {
				t.Errorf("countSpecialCharacters() = %v, want %v", got, tt.fields.wantSpecialCharacters)
			}
		})
	}
}
