package amount

import "testing"

func TestParseFormatKSC(t *testing.T) {
	cases := []struct {
		in  string
		sat int64
		out string
		bad bool
	}{
		{"0", 0, "0", false},
		{"1", 100000000, "1", false},
		{"0.00000001", 1, "0.00000001", false},
		{"0.1", 10000000, "0.1", false},
		{"100.12345678", 10012345678, "100.12345678", false},
		{"-0.5", -50000000, "-0.5", false},
		{"0.000000001", 0, "", true},
		{"abc", 0, "", true},
		{"", 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			sat, err := ParseKSC(tc.in)
			if tc.bad {
				if err == nil {
					t.Fatalf("expected error for %q", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sat != tc.sat {
				t.Fatalf("ParseKSC(%q) = %d, want %d", tc.in, sat, tc.sat)
			}
			if got := FormatKSC(sat); got != tc.out {
				t.Fatalf("FormatKSC(%d) = %q, want %q", sat, got, tc.out)
			}
		})
	}
}

func TestTokenAmount(t *testing.T) {
	raw, err := ParseTokenAmount("1.5", 18)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "1500000000000000000" {
		t.Fatalf("raw = %s", raw)
	}
	if got := FormatTokenAmount(raw, 18); got != "1.5" {
		t.Fatalf("formatted = %s", got)
	}
	if _, err := ParseTokenAmount("1.0001", 3); err == nil {
		t.Fatal("expected decimal overflow error")
	}
}
