// Copyright 2016 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package apd

import (
	"encoding/json"
	"fmt"
	"math"
	"math/bits"
	"strings"
	"testing"
	"unsafe"
)

var (
	testCtx = &BaseContext
)

func (d *Decimal) GoString() string {
	return fmt.Sprintf(`{Coeff: %s, Exponent: %d, Negative: %v, Form: %s}`, d.Coeff.String(), d.Exponent, d.Negative, d.Form)
}

// testExponentError skips t if err was caused by an exponent being outside
// of the package's supported exponent range. Since the exponent is so large,
// we don't support those tests yet (i.e., it's an expected failure, so we
// skip it).
func testExponentError(t *testing.T, err error) {
	if err == nil {
		return
	}
	if err.Error() == errExponentOutOfRangeStr {
		t.Skip(err)
	}
}

func newDecimal(t *testing.T, c *Context, s string) *Decimal {
	d, _, err := c.NewFromString(s)
	testExponentError(t, err)
	if err != nil {
		t.Fatalf("%s: %+v", s, err)
	}
	return d
}

func TestNewWithBigInt(t *testing.T) {
	tests := []string{
		"0",
		"1",
		"-1",
	}
	for _, tc := range tests {
		t.Run(tc, func(t *testing.T) {
			expect, _, err := new(Decimal).SetString(tc)
			if err != nil {
				t.Fatal(err)
			}
			b, ok := new(BigInt).SetString(tc, 10)
			if !ok {
				t.Fatal("bad bigint")
			}
			d := NewWithBigInt(b, 0)
			if d.Coeff.Sign() < 0 {
				t.Fatal("unexpected negative coeff")
			}
			// Verify that changing b doesn't change d.
			b.Set(NewBigInt(1234))
			if d.CmpTotal(expect) != 0 {
				t.Fatalf("expected %s, got %s", expect, d)
			}
		})
	}
}

func TestUpscale(t *testing.T) {
	tests := []struct {
		x, y *Decimal
		a, b *BigInt
		s    int32
	}{
		{x: New(1, 0), y: New(100, -1), a: NewBigInt(10), b: NewBigInt(100), s: -1},
		{x: New(1, 0), y: New(10, -1), a: NewBigInt(10), b: NewBigInt(10), s: -1},
		{x: New(1, 0), y: New(10, 0), a: NewBigInt(1), b: NewBigInt(10), s: 0},
		{x: New(1, 1), y: New(1, 0), a: NewBigInt(10), b: NewBigInt(1), s: 0},
		{x: New(10, -2), y: New(1, -1), a: NewBigInt(10), b: NewBigInt(10), s: -2},
		{x: New(1, -2), y: New(100, 1), a: NewBigInt(1), b: NewBigInt(100000), s: -2},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s, %s", tc.x, tc.y), func(t *testing.T) {
			a, b, s, err := upscale(tc.x, tc.y, new(BigInt))
			if err != nil {
				t.Fatal(err)
			}
			if a.Cmp(tc.a) != 0 {
				t.Errorf("a: expected %s, got %s", tc.a, a)
			}
			if b.Cmp(tc.b) != 0 {
				t.Errorf("b: expected %s, got %s", tc.b, b)
			}
			if s != tc.s {
				t.Errorf("s: expected %d, got %d", tc.s, s)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		x, y string
		r    string
	}{
		{x: "1", y: "10", r: "11"},
		{x: "1", y: "1e1", r: "11"},
		{x: "1e1", y: "1", r: "11"},
		{x: ".1e1", y: "100e-1", r: "11.0"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s, %s", tc.x, tc.y), func(t *testing.T) {
			x := newDecimal(t, testCtx, tc.x)
			y := newDecimal(t, testCtx, tc.y)
			d := new(Decimal)
			_, err := testCtx.Add(d, x, y)
			if err != nil {
				t.Fatal(err)
			}
			s := d.String()
			if s != tc.r {
				t.Fatalf("expected: %s, got: %s", tc.r, s)
			}
		})
	}
}

func TestCmp(t *testing.T) {
	tests := []struct {
		x, y string
		c    int
	}{
		{x: "1", y: "10", c: -1},
		{x: "1", y: "1e1", c: -1},
		{x: "1e1", y: "1", c: 1},
		{x: ".1e1", y: "100e-1", c: -1},

		{x: ".1e1", y: "100e-2", c: 0},
		{x: "1", y: ".1e1", c: 0},
		{x: "1", y: "1", c: 0},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s, %s", tc.x, tc.y), func(t *testing.T) {
			x := newDecimal(t, testCtx, tc.x)
			y := newDecimal(t, testCtx, tc.y)
			c := x.Cmp(y)
			if c != tc.c {
				t.Fatalf("expected: %d, got: %d", tc.c, c)
			}
		})
	}
}

func TestModf(t *testing.T) {
	tests := []struct {
		x string
		i string
		f string
	}{
		{x: "1", i: "1", f: "0"},
		{x: "1.0", i: "1", f: "0.0"},
		{x: "1.0e1", i: "10", f: "0"},
		{x: "1.0e2", i: "1.0E+2", f: "0"},
		{x: "1.0e-1", i: "0", f: "0.10"},
		{x: "1.0e-2", i: "0", f: "0.010"},
		{x: "1.1", i: "1", f: "0.1"},
		{x: "1234.56", i: "1234", f: "0.56"},
		{x: "1234.56e2", i: "123456", f: "0"},
		{x: "1234.56e4", i: "1.23456E+7", f: "0"},
		{x: "1234.56e-2", i: "12", f: "0.3456"},
		{x: "1234.56e-4", i: "0", f: "0.123456"},
		{x: "1234.56e-6", i: "0", f: "0.00123456"},
		{x: "123456e-8", i: "0", f: "0.00123456"},
		{x: ".123456e8", i: "1.23456E+7", f: "0"},

		{x: "-1", i: "-1", f: "-0"},
		{x: "-1.0", i: "-1", f: "-0.0"},
		{x: "-1.0e1", i: "-10", f: "-0"},
		{x: "-1.0e2", i: "-1.0E+2", f: "-0"},
		{x: "-1.0e-1", i: "-0", f: "-0.10"},
		{x: "-1.0e-2", i: "-0", f: "-0.010"},
		{x: "-1.1", i: "-1", f: "-0.1"},
		{x: "-1234.56", i: "-1234", f: "-0.56"},
		{x: "-1234.56e2", i: "-123456", f: "-0"},
		{x: "-1234.56e4", i: "-1.23456E+7", f: "-0"},
		{x: "-1234.56e-2", i: "-12", f: "-0.3456"},
		{x: "-1234.56e-4", i: "-0", f: "-0.123456"},
		{x: "-1234.56e-6", i: "-0", f: "-0.00123456"},
		{x: "-123456e-8", i: "-0", f: "-0.00123456"},
		{x: "-.123456e8", i: "-1.23456E+7", f: "-0"},
	}
	for _, tc := range tests {
		t.Run(tc.x, func(t *testing.T) {
			x := newDecimal(t, testCtx, tc.x)
			integ, frac := new(Decimal), new(Decimal)
			x.Modf(integ, frac)
			if tc.i != integ.String() {
				t.Fatalf("integ: expected: %s, got: %s", tc.i, integ)
			}
			if tc.f != frac.String() {
				t.Fatalf("frac: expected: %s, got: %s", tc.f, frac)
			}
			a := new(Decimal)
			if _, err := testCtx.Add(a, integ, frac); err != nil {
				t.Fatal(err)
			}
			if a.Cmp(x) != 0 {
				t.Fatalf("%s != %s", a, x)
			}
			if integ.Exponent < 0 {
				t.Fatal(integ.Exponent)
			}
			if frac.Exponent > 0 {
				t.Fatal(frac.Exponent)
			}

			integ2, frac2 := new(Decimal), new(Decimal)
			x.Modf(integ2, nil)
			x.Modf(nil, frac2)
			if integ.CmpTotal(integ2) != 0 {
				t.Fatalf("got %s, expected %s", integ2, integ)
			}
			if frac.CmpTotal(frac2) != 0 {
				t.Fatalf("got %s, expected %s", frac2, frac)
			}

			integ3, frac3 := new(Decimal).Set(x), new(Decimal)
			integ3.Modf(integ3, frac3)
			if integ.CmpTotal(integ3) != 0 || frac.CmpTotal(frac3) != 0 {
				t.Fatalf("integ aliased: got %s, %s, expected %s, %s", integ3, frac3, integ, frac)
			}
			integ4, frac4 := new(Decimal), new(Decimal).Set(x)
			frac4.Modf(integ4, frac4)
			if integ.CmpTotal(integ4) != 0 || frac.CmpTotal(frac4) != 0 {
				t.Fatalf("frac aliased: got %s, %s, expected %s, %s", integ4, frac4, integ, frac)
			}
			x5 := new(Decimal).Set(x)
			x5.Modf(x5, nil)
			if integ.CmpTotal(x5) != 0 {
				t.Fatalf("integ aliased, frac nil: got %s, expected %s", x5, integ)
			}
			x6 := new(Decimal).Set(x)
			x6.Modf(nil, x6)
			if frac.CmpTotal(x6) != 0 {
				t.Fatalf("frac aliased, integ nil: got %s, expected %s", x6, frac)
			}
		})
	}

	// Ensure we don't panic on both nil.
	a := new(Decimal)
	a.Modf(nil, nil)
}

func TestInt64(t *testing.T) {
	tests := []struct {
		x   string
		i   int64
		err bool
	}{
		{x: "0.12e1", err: true},
		{x: "0.1e1", i: 1},
		{x: "10", i: 10},
		{x: "12.3e3", i: 12300},
		{x: "1e-1", err: true},
		{x: "1e2", i: 100},
		{x: "1", i: 1},
		{x: "NaN", err: true},
		{x: "Inf", err: true},
		{x: "9223372036854775807", i: 9223372036854775807},
		{x: "-9223372036854775808", i: -9223372036854775808},
		{x: "9223372036854775808", err: true},
	}
	for _, tc := range tests {
		t.Run(tc.x, func(t *testing.T) {
			x := newDecimal(t, testCtx, tc.x)
			i, err := x.Int64()
			hasErr := err != nil
			if tc.err != hasErr {
				t.Fatalf("expected error: %v, got error: %v", tc.err, err)
			}
			if hasErr {
				return
			}
			if i != tc.i {
				t.Fatalf("expected: %v, got %v", tc.i, i)
			}
		})
	}
}

func TestQuoErr(t *testing.T) {
	tests := []struct {
		x, y string
		p    uint32
		err  string
	}{
		{x: "1", y: "1", p: 0, err: errZeroPrecisionStr},
		{x: "1", y: "0", p: 1, err: "division by zero"},
	}
	for _, tc := range tests {
		c := testCtx.WithPrecision(tc.p)
		x := newDecimal(t, testCtx, tc.x)
		y := newDecimal(t, testCtx, tc.y)
		d := new(Decimal)
		_, err := c.Quo(d, x, y)
		if err == nil {
			t.Fatal("expected error")
		}
		if err.Error() != tc.err {
			t.Fatalf("expected %s, got %s", tc.err, err)
		}
	}
}

func TestConditionString(t *testing.T) {
	tests := map[Condition]string{
		Overflow:             "overflow",
		Overflow | Underflow: "overflow, underflow",
		Subnormal | Inexact:  "inexact, subnormal",
	}
	for c, s := range tests {
		t.Run(s, func(t *testing.T) {
			cs := c.String()
			if cs != s {
				t.Errorf("expected %s; got %s", s, cs)
			}
		})
	}
}

func TestFloat64(t *testing.T) {
	tests := []float64{
		0,
		1,
		-1,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
		math.MaxFloat64,
		math.SmallestNonzeroFloat64,
	}

	for _, tc := range tests {
		t.Run(fmt.Sprint(tc), func(t *testing.T) {
			d := new(Decimal)
			d.SetFloat64(tc)
			f, err := d.Float64()
			if err != nil {
				t.Fatal(err)
			}
			if tc != f {
				t.Fatalf("expected %v, got %v", tc, f)
			}
		})
	}
}

func TestCeil(t *testing.T) {
	tests := map[float64]int64{
		0:     0,
		-0.1:  0,
		0.1:   1,
		-0.01: 0,
		0.01:  1,
		-0.9:  0,
		0.9:   1,
		-1:    -1,
		1:     1,
		-1.1:  -1,
		1.1:   2,
	}

	for f, r := range tests {
		t.Run(fmt.Sprint(f), func(t *testing.T) {
			d, err := new(Decimal).SetFloat64(f)
			if err != nil {
				t.Fatal(err)
			}
			sep := new(Decimal)
			_, err = testCtx.Ceil(sep, d)
			if err != nil {
				t.Fatal(err)
			}
			_, err = testCtx.Ceil(d, d)
			if err != nil {
				t.Fatal(err)
			}
			i, err := d.Int64()
			if err != nil {
				t.Fatal(err)
			}
			if i != r {
				t.Fatalf("got %v, expected %v", i, r)
			}
			if sep.CmpTotal(d) != 0 {
				t.Fatalf("aliased: got %s, expected %s", d, sep)
			}
		})
	}
}

func TestFloor(t *testing.T) {
	tests := map[float64]int64{
		0:     0,
		-0.1:  -1,
		0.1:   0,
		-0.01: -1,
		0.01:  0,
		-0.9:  -1,
		0.9:   0,
		-1:    -1,
		1:     1,
		-1.1:  -2,
		1.1:   1,
	}

	for f, r := range tests {
		t.Run(fmt.Sprint(f), func(t *testing.T) {
			d, err := new(Decimal).SetFloat64(f)
			if err != nil {
				t.Fatal(err)
			}
			sep := new(Decimal)
			_, err = testCtx.Floor(sep, d)
			if err != nil {
				t.Fatal(err)
			}
			_, err = testCtx.Floor(d, d)
			if err != nil {
				t.Fatal(err)
			}
			i, err := d.Int64()
			if err != nil {
				t.Fatal(err)
			}
			if i != r {
				t.Fatalf("got %v, expected %v", i, r)
			}
			if sep.CmpTotal(d) != 0 {
				t.Fatalf("aliased: got %s, expected %s", d, sep)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := map[string]struct {
		e, E, f, g, G string
	}{
		"NaN":       {},
		"Infinity":  {},
		"-Infinity": {},
		"sNaN":      {},
		"0": {
			e: "0e+0",
			E: "0E+0",
		},
		"-0": {
			e: "-0e+0",
			E: "-0E+0",
		},
		"0.0": {
			e: "0e-1",
			E: "0E-1",
		},
		"-0.0": {
			e: "-0e-1",
			E: "-0E-1",
		},
		"0E+2": {
			e: "0e+2",
			f: "000",
			g: "0e+2",
		},
		"0E-9": {
			e: "0e-9",
			f: "0.000000000",
			g: "0.000000000",
			G: "0.000000000",
		},
		"0E-2000": {
			e: "0e-2000",
			f: "0." + strings.Repeat("0", 2000),
			g: "0." + strings.Repeat("0", 2000),
			G: "0." + strings.Repeat("0", 2000),
		},
		"0E-2001": {
			e: "0e-2001",
			f: "0." + strings.Repeat("0", 2001),
			g: "0e-2001",
			G: "0E-2001",
		},
	}
	verbs := []string{"%e", "%E", "%f", "%g", "%G"}

	for input, tc := range tests {
		t.Run(input, func(t *testing.T) {
			d, _, err := NewFromString(input)
			if err != nil {
				t.Fatal(err)
			}
			for i, s := range []string{tc.e, tc.E, tc.f, tc.g, tc.G} {
				if s == "" {
					s = input
				}
				v := verbs[i]
				t.Run(v, func(t *testing.T) {
					out := fmt.Sprintf(v, d)
					if out != s {
						t.Fatalf("expected %s, got %s", s, out)
					}
				})
			}
		})
	}
}

// fmtAllowsPaddingAndMinusFlags is set to true if the %-0 flag combination is
// allowed. This combination was not allowed before Go 1.23, but it is allowed
// in Go 1.23 and later. See https://github.com/golang/go/issues/61784 for
// details.
var fmtAllowsPaddingAndMinusFlags bool

// The following type definition, customer formatter implementation, and init
// function are used to test for fmtAllowsPaddingAndMinusFlags.
type fmtAllowsPaddingAndMinusFlagsHelper struct{}

func (fmtAllowsPaddingAndMinusFlagsHelper) Format(s fmt.State, format rune) {
	fmtAllowsPaddingAndMinusFlags = s.Flag('-') && s.Flag('0')
}

func init() { fmt.Printf("%-0s", fmtAllowsPaddingAndMinusFlagsHelper{}) }

func TestFormatFlags(t *testing.T) {
	const stdD = "1.23E+56"
	tests := []struct {
		d   string
		fmt string
		out string
	}{
		{
			d:   stdD,
			fmt: "%3G",
			out: "1.23E+56",
		},
		{
			d:   stdD,
			fmt: "%010G",
			out: "001.23E+56",
		},
		{
			d:   stdD,
			fmt: "%10G",
			out: "  1.23E+56",
		},
		{
			d:   stdD,
			fmt: "%+G",
			out: "+1.23E+56",
		},
		{
			d:   stdD,
			fmt: "% G",
			out: " 1.23E+56",
		},
		{
			d:   stdD,
			fmt: "%-10G",
			out: "1.23E+56  ",
		},
		{
			d:   stdD,
			fmt: "%-010G",
			out: func() string {
				if !fmtAllowsPaddingAndMinusFlags {
					return "1.23E+56  "
				}
				return "001.23E+56"
			}(),
		},
		{
			d:   stdD,
			fmt: "%0-10G",
			out: func() string {
				if !fmtAllowsPaddingAndMinusFlags {
					return "1.23E+56  "
				}
				return "001.23E+56"
			}(),
		},
		{
			d:   "nan",
			fmt: "%-10G",
			out: "NaN       ",
		},
		{
			d:   "nan",
			fmt: "%10G",
			out: "       NaN",
		},
		{
			d:   "nan",
			fmt: "%010G",
			out: "       NaN",
		},
		{
			d:   "inf",
			fmt: "%-10G",
			out: "Infinity  ",
		},
		{
			d:   "inf",
			fmt: "%10G",
			out: "  Infinity",
		},
		{
			d:   "inf",
			fmt: "%010G",
			out: "  Infinity",
		},
		{
			d:   "-inf",
			fmt: "%-10G",
			out: "-Infinity ",
		},
		{
			d:   "-inf",
			fmt: "%10G",
			out: " -Infinity",
		},
		{
			d:   "-inf",
			fmt: "%010G",
			out: " -Infinity",
		},
		{
			d:   "0",
			fmt: "%d",
			out: "%!d(*apd.Decimal=0)",
		},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s: %s", tc.d, tc.fmt), func(t *testing.T) {
			d := newDecimal(t, &BaseContext, tc.d)
			s := fmt.Sprintf(tc.fmt, d)
			if s != tc.out {
				t.Fatalf("expected %q, got %q", tc.out, s)
			}
		})
	}
}

func TestContextSetStringt(t *testing.T) {
	tests := []struct {
		s      string
		c      *Context
		expect string
	}{
		{
			s:      "1.234",
			c:      &BaseContext,
			expect: "1.234",
		},
		{
			s:      "1.234",
			c:      BaseContext.WithPrecision(2),
			expect: "1.2",
		},
	}
	for i, tc := range tests {
		t.Run(fmt.Sprintf("%d: %s", i, tc.s), func(t *testing.T) {
			d := new(Decimal)
			if _, _, err := tc.c.SetString(d, tc.s); err != nil {
				t.Fatal(err)
			}
			got := d.String()
			if got != tc.expect {
				t.Fatalf("expected: %s, got: %s", tc.expect, got)
			}
		})
	}
}

func TestQuantize(t *testing.T) {
	tests := []struct {
		s      string
		e      int32
		expect string
	}{
		{
			s:      "1.00",
			e:      -1,
			expect: "1.0",
		},
		{
			s:      "2.0",
			e:      -1,
			expect: "2.0",
		},
		{
			s:      "3",
			e:      -1,
			expect: "3.0",
		},
		{
			s:      "9.9999",
			e:      -2,
			expect: "10.00",
		},
	}
	c := BaseContext.WithPrecision(10)
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s: %d", tc.s, tc.e), func(t *testing.T) {
			d, _, err := NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Quantize(d, d, tc.e); err != nil {
				t.Fatal(err)
			}
			s := d.String()
			if s != tc.expect {
				t.Fatalf("expected: %s, got: %s", tc.expect, s)
			}
		})
	}
}

func TestQuantizeRounding(t *testing.T) {
	tests := []struct {
		s      string
		e      int32
		rnd    Rounder
		expect string
		cond   Condition
	}{
		{s: "0.002", e: -1, rnd: RoundCeiling, expect: "0.1", cond: Inexact | Rounded},
		{s: "1.002", e: -1, rnd: RoundCeiling, expect: "1.1", cond: Inexact | Rounded},
		{s: "-0.002", e: -1, rnd: RoundCeiling, expect: "-0.0", cond: Inexact | Rounded},
		{s: "0.002", e: -1, rnd: RoundFloor, expect: "0.0", cond: Inexact | Rounded},
		{s: "-0.002", e: -1, rnd: RoundFloor, expect: "-0.1", cond: Inexact | Rounded},
		{s: "0.002", e: -1, rnd: RoundUp, expect: "0.1", cond: Inexact | Rounded},
		{s: "0.002", e: -1, rnd: RoundDown, expect: "0.0", cond: Inexact | Rounded},
		{s: "0.002", e: -1, rnd: Round05Up, expect: "0.1", cond: Inexact | Rounded},
		{s: "-0.002", e: -1, rnd: Round05Up, expect: "-0.1", cond: Inexact | Rounded},
		{s: "0.002", e: -1, rnd: RoundHalfDown, expect: "0.0", cond: Inexact | Rounded},
		{s: "0.04", e: -1, rnd: RoundHalfUp, expect: "0.0", cond: Inexact | Rounded},
		{s: "0.05", e: -1, rnd: RoundHalfUp, expect: "0.1", cond: Inexact | Rounded},
		{s: "0.05", e: -1, rnd: RoundHalfEven, expect: "0.0", cond: Inexact | Rounded},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s: %d %s", tc.s, tc.e, tc.rnd), func(t *testing.T) {
			c := Context{Precision: 100, Rounding: tc.rnd, MaxExponent: 1000, MinExponent: -1000, Traps: DefaultTraps}
			d, _, err := NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			res, err := c.Quantize(d, d, tc.e)
			if err != nil {
				t.Fatal(err)
			}
			if s := d.String(); s != tc.expect {
				t.Fatalf("expected: %s, got: %s", tc.expect, s)
			}
			if res != tc.cond {
				t.Fatalf("expected condition: %s, got: %s", tc.cond, res)
			}
			// Repeat with a separate destination; the input must be unchanged.
			x, _, err := NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			var sep Decimal
			if _, err := c.Quantize(&sep, x, tc.e); err != nil {
				t.Fatal(err)
			}
			if sep.CmpTotal(d) != 0 {
				t.Fatalf("expected: %s, got: %s", d, &sep)
			}
			if s := x.String(); s != tc.s {
				t.Fatalf("input changed: expected: %s, got: %s", tc.s, s)
			}
		})
	}
}

func TestRoundToIntegralRounding(t *testing.T) {
	tests := []struct {
		s      string
		rnd    Rounder
		expect string
		cond   Condition
	}{
		{s: "0.05", rnd: RoundCeiling, expect: "1", cond: Inexact | Rounded},
		{s: "-0.05", rnd: RoundFloor, expect: "-1", cond: Inexact | Rounded},
		{s: "0.002", rnd: Round05Up, expect: "1", cond: Inexact | Rounded},
		{s: "0.05", rnd: RoundHalfUp, expect: "0", cond: Inexact | Rounded},
		{s: "0.05", rnd: RoundDown, expect: "0", cond: Inexact | Rounded},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s: %s", tc.s, tc.rnd), func(t *testing.T) {
			c := Context{Precision: 100, Rounding: tc.rnd, MaxExponent: 1000, MinExponent: -1000, Traps: DefaultTraps}
			x, _, err := NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			var d Decimal
			res, err := c.RoundToIntegralValue(&d, x)
			if err != nil {
				t.Fatal(err)
			}
			if s := d.String(); s != tc.expect {
				t.Fatalf("RoundToIntegralValue: expected: %s, got: %s", tc.expect, s)
			}
			if res != 0 {
				t.Fatalf("RoundToIntegralValue: expected no condition, got: %s", res)
			}
			x, _, err = NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			res, err = c.RoundToIntegralExact(&d, x)
			if err != nil {
				t.Fatal(err)
			}
			if s := d.String(); s != tc.expect {
				t.Fatalf("RoundToIntegralExact: expected: %s, got: %s", tc.expect, s)
			}
			if res != tc.cond {
				t.Fatalf("RoundToIntegralExact: expected condition: %s, got: %s", tc.cond, res)
			}
		})
	}
}

func TestCmpOrder(t *testing.T) {
	tests := []struct {
		s     string
		order int
	}{
		{s: "-NaN", order: -4},
		{s: "-sNaN", order: -3},
		{s: "-Infinity", order: -2},
		{s: "-127", order: -1},
		{s: "-1.00", order: -1},
		{s: "-1", order: -1},
		{s: "-0.000", order: -1},
		{s: "-0", order: -1},
		{s: "0", order: 1},
		{s: "1.2300", order: 1},
		{s: "1.23", order: 1},
		{s: "1E+9", order: 1},
		{s: "Infinity", order: 2},
		{s: "sNaN", order: 3},
		{s: "NaN", order: 4},
	}

	for _, tc := range tests {
		t.Run(tc.s, func(t *testing.T) {
			d, _, err := NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			o := d.cmpOrder()
			if o != tc.order {
				t.Fatalf("got %d, expected %d", o, tc.order)
			}
		})
	}
}

func TestIsZero(t *testing.T) {
	tests := []struct {
		s    string
		zero bool
	}{
		{s: "-NaN", zero: false},
		{s: "-sNaN", zero: false},
		{s: "-Infinity", zero: false},
		{s: "-127", zero: false},
		{s: "-1.00", zero: false},
		{s: "-1", zero: false},
		{s: "-0.000", zero: true},
		{s: "-0", zero: true},
		{s: "0", zero: true},
		{s: "1.2300", zero: false},
		{s: "1.23", zero: false},
		{s: "1E+9", zero: false},
		{s: "Infinity", zero: false},
		{s: "sNaN", zero: false},
		{s: "NaN", zero: false},
	}

	for _, tc := range tests {
		t.Run(tc.s, func(t *testing.T) {
			d, _, err := NewFromString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			z := d.IsZero()
			if z != tc.zero {
				t.Fatalf("got %v, expected %v", z, tc.zero)
			}
		})
	}
}

func TestNeg(t *testing.T) {
	tests := map[string]string{
		"0":          "0",
		"-0":         "0",
		"-0.000":     "0.000",
		"-00.000100": "0.000100",
	}

	for tc, expect := range tests {
		t.Run(tc, func(t *testing.T) {
			d, _, err := NewFromString(tc)
			if err != nil {
				t.Fatal(err)
			}
			var z Decimal
			z.Neg(d)
			s := z.String()
			if s != expect {
				t.Fatalf("expected %s, got %s", expect, s)
			}
		})
	}
}

func TestReduce(t *testing.T) {
	tests := map[string]int{
		"-0":        0,
		"0":         0,
		"0.0":       0,
		"00":        0,
		"0.00":      0,
		"-01000":    3,
		"01000":     3,
		"-1":        0,
		"1":         0,
		"-10.000E4": 4,
		"10.000E4":  4,
		"-10.00":    3,
		"10.00":     3,
		"-10":       1,
		"10":        1,
		"-143200000000000000000000000000000000000000000000000000000000": 56,
		"143200000000000000000000000000000000000000000000000000000000":  56,
		"Inf": 0,
		"NaN": 0,
	}

	for s, n := range tests {
		t.Run(s, func(t *testing.T) {
			d, _, err := NewFromString(s)
			if err != nil {
				t.Fatal(err)
			}
			_, got := d.Reduce(d)
			if n != got {
				t.Fatalf("got %v, expected %v", got, n)
			}
		})
	}
}

// TestRootZeroTraps checks that a root of zero whose exponent is clamped
// into the context's range returns an error when Clamped is trapped. The
// GDA vectors cover the exponent and condition but cannot set Traps.
func TestRootZeroTraps(t *testing.T) {
	c := &Context{Precision: 4, MaxExponent: 9, MinExponent: -9, Rounding: RoundHalfUp, Traps: Clamped}
	tests := []struct {
		op   string
		root func(d, x *Decimal) (Condition, error)
		x    string
	}{
		{op: "sqrt", root: c.Sqrt, x: "0E-25"},
		{op: "sqrt", root: c.Sqrt, x: "0E+20"},
		{op: "cbrt", root: c.Cbrt, x: "0E-37"},
		{op: "cbrt", root: c.Cbrt, x: "0E+30"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s %s", tc.op, tc.x), func(t *testing.T) {
			x := newDecimal(t, testCtx, tc.x)
			d := new(Decimal)
			_, err := tc.root(d, x)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != Clamped.String() {
				t.Fatalf("expected %s, got %s", Clamped, err)
			}
		})
	}
}

func rootTestContext(p uint32, rounding Rounder) *Context {
	return &Context{
		Precision:   p,
		MaxExponent: 999,
		MinExponent: -999,
		Rounding:    rounding,
		Traps:       0,
	}
}

// TestRootSpecialValues checks NaNs, infinities, signed zeros, n == 0,
// n == 1 and zero-precision handling.
func TestRootSpecialValues(t *testing.T) {
	c := rootTestContext(9, RoundHalfUp)
	tests := []struct {
		n    uint32
		x    string
		want string
		cond Condition
		neg  bool
	}{
		// Quiet NaNs propagate without conditions; signaling NaNs become quiet
		// with InvalidOperation.
		{2, "NaN", "NaN", 0, false},
		{7, "NaN123", "NaN123", 0, false},
		{2, "sNaN", "NaN", InvalidOperation, false},
		{3, "-sNaN", "-NaN", InvalidOperation, true},
		// Infinities.
		{2, "Infinity", "Infinity", 0, false},
		{3, "Infinity", "Infinity", 0, false},
		{3, "-Infinity", "-Infinity", 0, true},
		{5, "-Infinity", "-Infinity", 0, true},
		{2, "-Infinity", "NaN", InvalidOperation, false},
		{4, "-Infinity", "NaN", InvalidOperation, false},
		// Signed zeros keep their sign; ideal exponent is floor(E/n).
		{2, "0", "0", 0, false},
		{2, "-0", "-0", 0, true},
		{2, "0E+5", "0E+2", 0, false},
		{2, "-0E+11", "-0E+5", 0, true},
		{2, "0E-5", "0E-3", 0, false},
		{3, "-0E+10", "-0E+3", 0, true},
		// n == 1 applies to specials as well.
		{1, "Infinity", "Infinity", 0, false},
		{1, "-Infinity", "-Infinity", 0, true},
		{1, "sNaN", "NaN", InvalidOperation, false},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d:%s", tc.n, tc.x), func(t *testing.T) {
			x := newDecimal(t, testCtx, tc.x)
			d := new(Decimal)
			res, err := c.Root(d, x, tc.n)
			if err != nil {
				t.Fatalf("unexpected error: %+v", err)
			}
			if res != tc.cond {
				t.Fatalf("conditions: got %s (%d), want %s (%d)", res, res, tc.cond, tc.cond)
			}
			want := newDecimal(t, testCtx, tc.want)
			if d.Form == Finite {
				if d.Cmp(want) != 0 || d.Negative != tc.neg {
					t.Fatalf("got %s, want %s", d, want)
				}
				if d.IsZero() && d.Exponent != want.Exponent {
					t.Fatalf("zero exponent: got %d, want %d", d.Exponent, want.Exponent)
				}
			} else if d.CmpTotal(want) != 0 {
				t.Fatalf("got %s, want %s", d, want)
			}
		})
	}

	// n == 0 is always InvalidOperation, even for infinities and NaNs handled
	// above; with the default traps it becomes an error.
	trapCtx := rootTestContext(9, RoundHalfUp)
	trapCtx.Traps = InvalidOperation
	for _, x := range []string{"128", "Infinity", "-Infinity", "0"} {
		d := new(Decimal)
		_, err := trapCtx.Root(d, newDecimal(t, testCtx, x), 0)
		if err == nil || err.Error() != InvalidOperation.String() {
			t.Fatalf("root(0, %s): got %v, want %s error", x, err, InvalidOperation)
		}
		if d.Form != NaN {
			t.Fatalf("root(0, %s): want NaN, got %s", x, d)
		}
	}

	// Even roots of negative numbers are invalid; odd roots are fine.
	d := new(Decimal)
	if _, err := c.Root(d, newDecimal(t, testCtx, "-8"), 4); err != nil {
		t.Fatal(err)
	} else if d.Form != NaN {
		t.Fatalf("4th root of -8: want NaN, got %s", d)
	}
	if _, err := c.Root(d, newDecimal(t, testCtx, "-3125"), 5); err != nil {
		t.Fatal(err)
	} else if d.String() != "-5" {
		t.Fatalf("5th root of -3125: got %s", d)
	}

	// Zero Precision returns an ordinary error and does not panic.
	c0 := &Context{Precision: 0, MaxExponent: MaxExponent, MinExponent: MinExponent}
	if _, err := c0.Root(new(Decimal), New(2, 0), 3); err == nil {
		t.Fatal("expected error with zero precision")
	}
	// Specials still work with zero precision.
	if _, err := c0.Root(d, newDecimal(t, testCtx, "-0E-5"), 2); err != nil {
		t.Fatalf("zero operand with zero precision: %+v", err)
	}

	// n == 1 is context rounding of x.
	for _, rnd := range []Rounder{RoundDown, RoundHalfUp, RoundHalfEven, RoundCeiling, RoundFloor, RoundHalfDown, RoundUp, Round05Up} {
		c1 := rootTestContext(3, rnd)
		for _, xs := range []string{"1.2345", "-1.2355", "2", "9.999"} {
			x := newDecimal(t, testCtx, xs)
			got := new(Decimal)
			r1, err := c1.Root(got, x, 1)
			if err != nil {
				t.Fatal(err)
			}
			want := new(Decimal)
			r2 := c1.round(want, x)
			if r1 != r2 || got.CmpTotal(want) != 0 {
				t.Fatalf("root(1,%s) [%s]: got %s (%s), want %s (%s)", xs, rnd, got, r1, want, r2)
			}
		}
	}
}

// TestRootZeroClamp checks that a zero whose ideal exponent is outside the
// context range is clamped, following the zero exponent rules.
func TestRootZeroClamp(t *testing.T) {
	c := &Context{Precision: 4, MaxExponent: 9, MinExponent: -9, Rounding: RoundHalfUp, Traps: 0}
	tests := []struct {
		n    uint32
		x    string
		want string
		cond Condition
	}{
		{2, "0E-25", "0E-12", Clamped}, // floor(-25/2) = -13, Etiny = -12
		{3, "0E-37", "0E-12", Clamped}, // floor(-37/3) = -13
		{7, "0", "0", 0},
		{2, "0E+20", "0E+9", Clamped},
		{3, "-0E+30", "-0E+9", Clamped},
	}
	for _, tc := range tests {
		x := newDecimal(t, testCtx, tc.x)
		d := new(Decimal)
		res, err := c.Root(d, x, tc.n)
		if err != nil {
			t.Fatal(err)
		}
		if res != tc.cond {
			t.Fatalf("root(%d, %s): got flags %s, want %s", tc.n, tc.x, res, tc.cond)
		}
		want := newDecimal(t, testCtx, tc.want)
		if d.Cmp(want) != 0 || d.Negative != want.Negative || d.Exponent != want.Exponent {
			t.Fatalf("root(%d, %s): got %s (exp %d), want %s (exp %d)", tc.n, tc.x, d, d.Exponent, want, want.Exponent)
		}
	}
}

// TestRootExact verifies that terminating roots are exact (no spurious
// Inexact), that dropping only zero digits reports Rounded alone, and that
// non-perfect powers report Inexact and Rounded.
func TestRootExact(t *testing.T) {
	c := rootTestContext(20, RoundHalfUp)
	exact := []struct {
		n uint32
		x string
		y string
	}{
		{7, "128", "2"},     // 2**7
		{10, "1024", "2"},   // 2**10
		{5, "3125", "5"},    // 5**5
		{4, "625", "5"},     // 5**4
		{6, "64", "2"},      // 2**6
		{7, "-128", "-2"},   // negative odd
		{3, "3.375", "1.5"}, // (1.5)**3
		{7, "610.3515625", "2.5"},
		{2, "0.000000000001", "0.000001"},
		{3, "0.000000000001", "0.0001"},
		{7, "1.28E2", "2"}, // 1.28E2 == 128 == 2**7
	}
	for _, tc := range exact {
		x := newDecimal(t, testCtx, tc.x)
		d := new(Decimal)
		res, err := c.Root(d, x, tc.n)
		if err != nil {
			t.Fatal(err)
		}
		want := newDecimal(t, testCtx, tc.y)
		if d.Cmp(want) != 0 || d.Negative != want.Negative {
			t.Fatalf("root(%d, %s): got %s, want %s", tc.n, tc.x, d, want)
		}
		if res != 0 {
			t.Fatalf("root(%d, %s) = %s: expected no conditions, got %s", tc.n, tc.x, d, res)
		}
	}

	// Dropping only zero digits: Rounded but not Inexact.
	c1 := rootTestContext(1, RoundHalfUp)
	for _, tc := range []struct {
		n uint32
		x string
		y string
	}{
		{2, "1.00E-2", "0.1"},
		{2, "100", "1E+1"},
		{3, "1000.00", "1E+1"},
	} {
		x := newDecimal(t, testCtx, tc.x)
		d := new(Decimal)
		res, err := c1.Root(d, x, tc.n)
		if err != nil {
			t.Fatal(err)
		}
		want := newDecimal(t, testCtx, tc.y)
		if d.Cmp(want) != 0 {
			t.Fatalf("root(%d, %s): got %s, want %s", tc.n, tc.x, d, want)
		}
		if res != Rounded {
			t.Fatalf("root(%d, %s): got %s, want Rounded only", tc.n, tc.x, res)
		}
	}

	// Non-perfect powers are Inexact and Rounded. The expected values are
	// also verified against the exact rounding boundaries by
	// TestRootRandomCorrectness.
	for _, tc := range []struct {
		n uint32
		p uint32
		x string
		y string
	}{
		{2, 20, "2", "1.41421356237309504880"},
		{7, 16, "2", "1.104089513673812"},
		{100, 16, "2", "1.006955550056719"},
		{3, 20, "-2", "-1.2599210498948731648"},
	} {
		cc := rootTestContext(tc.p, RoundHalfUp)
		x := newDecimal(t, testCtx, tc.x)
		d := new(Decimal)
		res, err := cc.Root(d, x, tc.n)
		if err != nil {
			t.Fatal(err)
		}
		want := newDecimal(t, testCtx, tc.y)
		if d.Cmp(want) != 0 || d.Negative != want.Negative {
			t.Fatalf("root(%d, %s): got %s, want %s", tc.n, tc.x, d, want)
		}
		if res != Inexact|Rounded {
			t.Fatalf("root(%d, %s): got %s, want inexact,rounded", tc.n, tc.x, res)
		}
	}
}

// TestRootRoundingModes checks that every Rounder selects the correct
// neighbor on each side of, and exactly at, a tie, for positive and
// negative operands.
func TestRootRoundingModes(t *testing.T) {
	// 7th root of 610.3515625 is exactly 2.5: a tie at precision 1.
	// 7th root of 2 ~= 1.10409 (below the half), 7th root of 3 ~= 1.16993
	// (above the half) at precision 2.
	type tc struct {
		x    string
		p    uint32
		want map[Rounder]string
	}
	tests := []tc{
		{
			x: "610.3515625", p: 1,
			want: map[Rounder]string{
				RoundDown: "2", RoundHalfUp: "3", RoundHalfEven: "2",
				RoundCeiling: "3", RoundFloor: "2", RoundHalfDown: "2",
				RoundUp: "3", Round05Up: "2",
			},
		},
		{
			x: "-610.3515625", p: 1,
			want: map[Rounder]string{
				RoundDown: "-2", RoundHalfUp: "-3", RoundHalfEven: "-2",
				RoundCeiling: "-2", RoundFloor: "-3", RoundHalfDown: "-2",
				RoundUp: "-3", Round05Up: "-2",
			},
		},
		{
			x: "2", p: 2,
			want: map[Rounder]string{
				RoundDown: "1.1", RoundHalfUp: "1.1", RoundHalfEven: "1.1",
				RoundCeiling: "1.2", RoundFloor: "1.1", RoundHalfDown: "1.1",
				RoundUp: "1.2", Round05Up: "1.1",
			},
		},
		{
			x: "3", p: 2,
			want: map[Rounder]string{
				RoundDown: "1.1", RoundHalfUp: "1.2", RoundHalfEven: "1.2",
				RoundCeiling: "1.2", RoundFloor: "1.1", RoundHalfDown: "1.2",
				RoundUp: "1.2", Round05Up: "1.1",
			},
		},
	}
	for _, tci := range tests {
		x := newDecimal(t, testCtx, tci.x)
		for rnd, wantS := range tci.want {
			c := rootTestContext(tci.p, rnd)
			d := new(Decimal)
			if _, err := c.Root(d, x, 7); err != nil {
				t.Fatal(err)
			}
			want := newDecimal(t, testCtx, wantS)
			if d.Cmp(want) != 0 || d.Negative != want.Negative {
				t.Fatalf("root(7,%s) [%s]: got %s, want %s", tci.x, rnd, d, want)
			}
		}
	}
}

// TestRootRange checks overflow and subnormal/underflow behavior against
// the context's exponent semantics.
func TestRootRange(t *testing.T) {
	// Overflow: the exact root 1E+10 exceeds Emax 9.
	c := &Context{Precision: 5, MaxExponent: 9, MinExponent: -9, Rounding: RoundHalfEven, Traps: 0}
	x := newDecimal(t, testCtx, "1E+20")
	d := new(Decimal)
	res, err := c.Root(d, x, 2)
	if err != nil {
		t.Fatal(err)
	}
	if d.Form != Infinite || d.Negative {
		t.Fatalf("overflow: got %s", d)
	}
	if res != Overflow|Inexact|Rounded {
		t.Fatalf("overflow flags: got %s, want overflow,inexact,rounded", res)
	}

	// Subnormal, inexact: mirrors GDA sqtx804.
	c11 := &Context{Precision: 11, MaxExponent: 9, MinExponent: -9, Rounding: RoundHalfUp, Traps: 0}
	x = newDecimal(t, testCtx, "1E-19")
	d = new(Decimal)
	res, err = c11.Root(d, x, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := newDecimal(t, testCtx, "3.162277660E-10"); d.Cmp(want) != 0 {
		t.Fatalf("subnormal inexact: got %s, want %s", d, want)
	}
	if res != Underflow|Subnormal|Inexact|Rounded {
		t.Fatalf("subnormal inexact flags: got %s", res)
	}

	// Subnormal, exact: mirrors GDA sqtx807.
	c12 := &Context{Precision: 12, MaxExponent: 9, MinExponent: -9, Rounding: RoundHalfUp, Traps: 0}
	x = newDecimal(t, testCtx, "1E-20")
	d = new(Decimal)
	res, err = c12.Root(d, x, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := newDecimal(t, testCtx, "1E-10"); d.Cmp(want) != 0 || d.Exponent != want.Exponent {
		t.Fatalf("subnormal exact: got %s, want %s", d, want)
	}
	if res != Subnormal {
		t.Fatalf("subnormal exact flags: got %s, want subnormal", res)
	}

	// A tiny root that rounds away to zero under RoundDown becomes a
	// clamped signed zero with Underflow.
	x = newDecimal(t, testCtx, "1E-40") // root = 1E-20, far below Etiny -19 at P11
	d = new(Decimal)
	res, err = c11.Root(d, x, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !d.IsZero() || d.Exponent != c11.etiny() {
		t.Fatalf("tiny-to-zero: got %s", d)
	}
	if res&(Underflow|Subnormal|Inexact|Clamped|Rounded) != Underflow|Subnormal|Inexact|Clamped|Rounded {
		t.Fatalf("tiny-to-zero flags: got %s", res)
	}

	// RoundUp of the same tiny root rounds up to the smallest step.
	cUp := &Context{Precision: 11, MaxExponent: 9, MinExponent: -9, Rounding: RoundUp, Traps: 0}
	d = new(Decimal)
	res, err = cUp.Root(d, x, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := newDecimal(t, testCtx, "1E-19"); d.Cmp(want) != 0 || d.Exponent != want.Exponent {
		t.Fatalf("tiny round up: got %s, want %s", d, want)
	}
}

// TestRootMisc covers large degrees, the default rounder, large exact
// powers, and trap handling.
func TestRootMisc(t *testing.T) {
	// Large degrees complete quickly and round correctly.
	c12 := rootTestContext(12, RoundHalfEven)
	for _, tc := range []struct {
		n    uint32
		want string
	}{
		{1000, "1.00069338746"},
		{10000, "1.00006931712"},
	} {
		d := new(Decimal)
		res, err := c12.Root(d, New(2, 0), tc.n)
		if err != nil {
			t.Fatal(err)
		}
		want := newDecimal(t, testCtx, tc.want)
		if d.Cmp(want) != 0 || res != Inexact|Rounded {
			t.Fatalf("root(2,%d) = %s (%s), want %s", tc.n, d, res, want)
		}
	}

	// An empty Rounding is RoundHalfUp: sqrt(2) at P2 and the exact tie
	// sqrt(625)=25 at P1.
	cd := &Context{Precision: 2, MaxExponent: 999, MinExponent: -999}
	d := new(Decimal)
	if _, err := cd.Root(d, New(2, 0), 2); err != nil {
		t.Fatal(err)
	}
	if d.String() != "1.4" {
		t.Fatalf("default rounding: %s", d)
	}
	cd1 := &Context{Precision: 1, MaxExponent: 999, MinExponent: -999}
	if _, err := cd1.Root(d, New(625, 0), 2); err != nil {
		t.Fatal(err)
	}
	if d.String() != "3E+1" {
		t.Fatalf("default rounding tie: %s", d)
	}

	// Large exact power: (1234567)**8.
	var x Decimal
	var nb BigInt
	nb.SetInt64(8)
	if _, err := BaseContext.integerPower(&x, New(1234567, 0), &nb); err != nil {
		t.Fatal(err)
	}
	c20 := rootTestContext(20, RoundHalfEven)
	d = new(Decimal)
	res, err := c20.Root(d, &x, 8)
	if err != nil {
		t.Fatal(err)
	}
	if d.Cmp(New(1234567, 0)) != 0 || res != 0 {
		t.Fatalf("exact 8th root: %s (%s)", d, res)
	}

	// Subnormal trap.
	ct := &Context{Precision: 11, MaxExponent: 9, MinExponent: -9, Traps: Subnormal}
	if _, err := ct.Root(new(Decimal), New(1, -19), 2); err == nil || err.Error() != Subnormal.String() {
		t.Fatalf("subnormal trap: %v", err)
	}
}

// TestRootAliasing allows d to alias x, leaves non-aliased inputs
// unchanged, and returns identical results on repeated and independent
// calls.
func TestRootAliasing(t *testing.T) {
	c := rootTestContext(7, RoundHalfEven)
	for _, n := range []uint32{2, 3, 5, 7, 25} {
		x := newDecimal(t, testCtx, "12345.6789")
		xs := new(Decimal).Set(x)
		// d aliases x.
		res1, err := c.Root(x, x, n)
		if err != nil {
			t.Fatal(err)
		}
		// Independent call with separate decimals.
		x2 := newDecimal(t, testCtx, "12345.6789")
		d2 := new(Decimal)
		res2, err := c.Root(d2, x2, n)
		if err != nil {
			t.Fatal(err)
		}
		if x.CmpTotal(d2) != 0 || res1 != res2 {
			t.Fatalf("n=%d: aliased %s (%s) != independent %s (%s)", n, x, res1, d2, res2)
		}
		// Non-aliased input is not modified.
		x3 := newDecimal(t, testCtx, "12345.6789")
		d3 := new(Decimal)
		if _, err := c.Root(d3, x3, n); err != nil {
			t.Fatal(err)
		}
		if x3.CmpTotal(xs) != 0 {
			t.Fatalf("n=%d: input changed to %s", n, x3)
		}
		// Repeated calls agree.
		d4 := new(Decimal)
		if _, err := c.Root(d4, x3, n); err != nil {
			t.Fatal(err)
		}
		if d4.CmpTotal(d3) != 0 {
			t.Fatalf("n=%d: repeated calls differ", n)
		}
	}
}

// rootGridSign returns sign(|x| - k**n * 10**(n*e)).
func rootGridSign(x *Decimal, k *BigInt, e int64, n uint32) int {
	var cAbs, kn BigInt
	cAbs.Abs(&x.Coeff)
	bigIntPow(&kn, k, int64(n))
	return cmpScaledInt(&cAbs, int64(x.Exponent), &kn, int64(n)*e)
}

// TestRootRandomCorrectness verifies rounding of randomly generated roots
// against the exact rounding boundaries, using integer nth-power
// comparisons, for every Rounder and several degrees and precisions.
func TestRootRandomCorrectness(t *testing.T) {
	rounders := []Rounder{RoundDown, RoundHalfUp, RoundHalfEven, RoundCeiling, RoundFloor, RoundHalfDown, RoundUp, Round05Up}
	degrees := []uint32{2, 3, 5, 7}
	// xorshift64* PRNG.
	var rng uint64 = 0x9e3779b97f4a7c15
	rand := func() uint64 {
		rng ^= rng >> 12
		rng ^= rng << 25
		rng ^= rng >> 27
		return rng * 0x2545F4914F6CDD1D
	}
	for iter := 0; iter < 600; iter++ {
		n := degrees[rand()%uint64(len(degrees))]
		coeff := int64(rand()%10_000_000 + 1)
		exp := int32(rand()%7 - 3)
		p := uint32(1 + rand()%6)
		neg := n%2 == 1 && rand()%2 == 0
		x := New(coeff, exp)
		if neg {
			x.Negative = true
		}
		// An equivalent input with trailing zeros must round identically.
		xz := new(Decimal)
		xz.Set(x)
		xz.Coeff.Mul(&xz.Coeff, NewBigInt(100))
		xz.Exponent -= 2

		for _, rnd := range rounders {
			c := rootTestContext(p, rnd)
			for _, in := range []*Decimal{x, xz} {
				// Determine whether the root is a terminating decimal (exact);
				// those cases are covered by the perfect-power checks below and
				// by c.round's own rounding tests.
				q := floorDiv(int64(in.Exponent), int64(n))
				rm := int64(in.Exponent) - q*int64(n)
				var Min, tmpM, Ai, Ain BigInt
				Min.Abs(&in.Coeff)
				if rm > 0 {
					Min.Mul(&Min, tableExp10(rm, &tmpM))
				}
				integerNthRoot(&Ai, &Min, n)
				bigIntPow(&Ain, &Ai, int64(n))
				inputExact := Ain.Cmp(&Min) == 0

				d := new(Decimal)
				res, err := c.Root(d, in, n)
				if err != nil {
					t.Fatalf("root(%d,%s) p=%d %s: %+v", n, in, p, rnd, err)
				}
				if inputExact {
					// Exact branch: rounding the terminating root must never
					// claim it was Inexact.
					if res&Inexact != 0 {
						t.Fatalf("exact root(%d,%s) p=%d %s = %s: spurious Inexact (%s)", n, in, p, rnd, d, res)
					}
					continue
				}
				if d.Form != Finite || res != Inexact|Rounded {
					t.Fatalf("root(%d,%s) p=%d %s = %s (%s)", n, in, p, rnd, d, res)
				}
				if d.Negative != neg {
					t.Fatalf("root(%d,%s) sign: got %s", n, in, d)
				}
				if d.NumDigits() > int64(p) {
					t.Fatalf("root(%d,%s) p=%d has %d digits: %s", n, in, p, d.NumDigits(), d)
				}
				var K BigInt
				K.Abs(&d.Coeff)
				e := int64(d.Exponent)
				var km1, kp1 BigInt
				km1.Sub(&K, bigOne)
				kp1.Add(&K, bigOne)
				// Sign-directed expectations on the magnitude.
				away := rnd == RoundUp || (rnd == RoundCeiling && !neg) || (rnd == RoundFloor && neg)
				toward := rnd == RoundDown || (rnd == RoundFloor && !neg) || (rnd == RoundCeiling && neg)
				nearest := rnd == RoundHalfUp || rnd == RoundHalfEven || rnd == RoundHalfDown
				// 05up rounds away from zero when the last digit of the
				// truncated (pre-rounding) coefficient is 0 or 5, and towards
				// zero otherwise. Recover the truncated coefficient: if the
				// result was incremented, root < K*10**e.
				if rnd == Round05Up {
					var k0, rem, zero BigInt
					k0.Set(&K)
					if rootGridSign(in, &K, e, n) < 0 {
						k0.Sub(&k0, bigOne)
					}
					rem.Rem(&k0, bigTen)
					if rem.Cmp(&zero) == 0 || rem.Cmp(bigFive) == 0 {
						away = true
					} else {
						toward = true
					}
				}
				switch {
				case away:
					// root magnitude in (K-1, K].
					if g := rootGridSign(in, &km1, e, n); g <= 0 {
						t.Fatalf("root(%d,%s) %s = %s: x <= (K-1)**n", n, in, rnd, d)
					}
					if g := rootGridSign(in, &K, e, n); g > 0 {
						t.Fatalf("root(%d,%s) %s = %s: x > K**n", n, in, rnd, d)
					}
				case toward:
					// root magnitude in [K, K+1), strictly inside (inexact).
					if g := rootGridSign(in, &K, e, n); g <= 0 {
						t.Fatalf("root(%d,%s) %s = %s: x <= K**n", n, in, rnd, d)
					}
					if g := rootGridSign(in, &kp1, e, n); g >= 0 {
						t.Fatalf("root(%d,%s) %s = %s: x >= (K+1)**n", n, in, rnd, d)
					}
				case nearest:
					// The magnitude lies strictly between K-1/2 and K+1/2.
					if g := rootHalfSign(&in.Coeff, int64(in.Exponent), &km1, e, n); g <= 0 {
						t.Fatalf("root(%d,%s) %s = %s: below K-1/2 midpoint", n, in, rnd, d)
					}
					if g := rootHalfSign(&in.Coeff, int64(in.Exponent), &K, e, n); g >= 0 {
						t.Fatalf("root(%d,%s) %s = %s: above K+1/2 midpoint", n, in, rnd, d)
					}
				}
			}
		}
	}

	// Perfect powers are exact with no flags, including odd negative powers.
	c := rootTestContext(10, RoundHalfEven)
	for iter := 0; iter < 200; iter++ {
		n := degrees[rand()%uint64(len(degrees))]
		b := New(int64(rand()%9999+1), int32(rand()%2)-1)
		var x Decimal
		var nb BigInt
		nb.SetInt64(int64(n))
		// Build the power without rounding so that x is exactly b**n.
		if _, err := BaseContext.integerPower(&x, b, &nb); err != nil {
			t.Fatal(err)
		}
		neg := n%2 == 1 && rand()%2 == 0
		if neg {
			x.Negative = true
		}
		d := new(Decimal)
		res, err := c.Root(d, &x, n)
		if err != nil {
			t.Fatal(err)
		}
		var absD Decimal
		absD.Abs(d)
		if absD.Cmp(b) != 0 || d.Negative != neg {
			t.Fatalf("exact root(%d, %s): got %s, want magnitude %s (neg=%v)", n, &x, d, b, neg)
		}
		if res != 0 {
			t.Fatalf("exact root(%d, %s) = %s: unexpected flags %s", n, &x, d, res)
		}
	}
}

// TestRootGDAVectors replays the GDA squareroot vectors through Root with
// n == 2 and the apd cuberoot vectors with n == 3, checking both result and
// conditions.
func TestRootGDAVectors(t *testing.T) {
	for _, file := range []string{"squareroot", "cuberoot-apd"} {
		t.Run(file, func(t *testing.T) {
			_, tcs := readGDA(t, file)
			for _, tc := range tcs {
				if GDAignore[tc.ID] || tc.HasNull() || tc.Result == "?" {
					continue
				}
				if GDAignoreFlags[tc.ID] {
					continue
				}
				degree := uint32(2)
				if tc.Operation == "cuberoot" {
					degree = 3
				} else if tc.Operation != "squareroot" {
					continue
				}
				t.Run(tc.ID, func(t *testing.T) {
					c := tc.Context(t)
					opctx := c.WithPrecision(1000)
					opctx.MaxExponent = MaxExponent
					opctx.MinExponent = MinExponent
					x := newDecimal(t, opctx, tc.Operands[0])
					d := new(Decimal)
					res, err := c.Root(d, x, degree)
					if err != nil {
						t.Fatalf("%+v", err)
					}
					var wantCond Condition
					for _, cond := range tc.Conditions {
						switch cond {
						case "underflow":
							wantCond |= Underflow
						case "inexact":
							wantCond |= Inexact
						case "overflow":
							wantCond |= Overflow
						case "subnormal":
							wantCond |= Subnormal
						case "invalid_operation":
							wantCond |= InvalidOperation
						case "rounded":
							wantCond |= Rounded
						case "clamped":
							wantCond |= Clamped
						case "invalid_context":
						default:
							t.Fatalf("unknown condition: %s", cond)
						}
					}
					if res != wantCond {
						t.Fatalf("flags: got %s (%d), want %s (%d)", res, res, wantCond, wantCond)
					}
					want := newDecimal(t, testCtx, tc.Result)
					if d.Form == Finite {
						if d.Cmp(want) != 0 || d.Negative != want.Negative {
							t.Fatalf("got %s, want %s", d, want)
						}
						if d.IsZero() && !GDAignoreZeroExponent[tc.ID] && d.Exponent != want.Exponent {
							t.Fatalf("zero exponent: got %d, want %d", d.Exponent, want.Exponent)
						}
					} else if d.CmpTotal(want) != 0 {
						t.Fatalf("got %s, want %s", d, want)
					}
				})
			}
		})
	}
}

// TestSizeof is meant to catch changes that unexpectedly increase
// the size of the BigInt, Decimal, and Context structs.
func TestSizeof(t *testing.T) {
	// map[uint_size][type]sizeof
	exp := map[int]map[string]uintptr{
		32: {
			"BigInt":  20,
			"Decimal": 28,
			"Context": 24,
		},
		64: {
			"BigInt":  24,
			"Decimal": 32,
			"Context": 32,
		},
	}[bits.UintSize]

	var b BigInt
	if s := unsafe.Sizeof(b); s != exp["BigInt"] {
		t.Errorf("sizeof(BigInt) changed: %d", s)
	}
	var d Decimal
	if s := unsafe.Sizeof(d); s != exp["Decimal"] {
		t.Errorf("sizeof(Decimal) changed: %d", s)
	}
	var c Context
	if s := unsafe.Sizeof(c); s != exp["Context"] {
		t.Errorf("sizeof(Context) changed: %d", s)
	}
}

// TestSize tests the Size method on BigInt and Decimal. Unlike Sizeof, which
// returns the shallow size of the structs, the Size method reports the total
// memory footprint of each struct and all referenced objects.
func TestSize(t *testing.T) {
	// map[uint_size][is_inline][type]size
	exp := map[int]map[bool]map[string]uintptr{
		32: {
			true: {
				"BigInt":  20,
				"Decimal": 28,
			},
			false: {
				"BigInt":  72,
				"Decimal": 80,
			},
		},
		64: {
			true: {
				"BigInt":  24,
				"Decimal": 32,
			},
			false: {
				"BigInt":  112,
				"Decimal": 120,
			},
		},
	}[bits.UintSize]

	var d Decimal
	if e, s := exp[true]["Decimal"], d.Size(); e != s {
		t.Errorf("(*Decimal).Size() != %d: %d", e, s)
	}
	if e, s := exp[true]["BigInt"], d.Coeff.Size(); e != s {
		t.Errorf("(*BigInt).Size() != %d: %d", e, s)
	}
	// Set to an inlinable value.
	d.SetInt64(1234)
	if e, s := exp[true]["Decimal"], d.Size(); e != s {
		t.Errorf("(*Decimal).Size() != %d: %d", e, s)
	}
	if e, s := exp[true]["BigInt"], d.Coeff.Size(); e != s {
		t.Errorf("(*BigInt).Size() != %d: %d", e, s)
	}
	// Set to a non-inlinable value.
	if _, _, err := d.SetString("123456789123456789123456789.123456789123456789"); err != nil {
		t.Fatal(err)
	}
	if d.Coeff.isInline() {
		// Sanity-check, in case inlineWords changes.
		t.Fatal("BigInt inlined large value. Did inlineWords change?")
	}
	if e, s := exp[false]["Decimal"], d.Size(); e != s {
		t.Errorf("(*Decimal).Size() != %d: %d", e, s)
	}
	if e, s := exp[false]["BigInt"], d.Coeff.Size(); e != s {
		t.Errorf("(*BigInt).Size() != %d: %d", e, s)
	}
}

func TestJSONEncoding(t *testing.T) {
	var encodingTests = []string{
		"0",
		"1",
		"2",
		"10",
		"1000",
		"1234567890",
		"298472983472983471903246121093472394872319615612417471234712061",
		"0.0",
		"NaN",
		"Inf",
		"123.456",
		"1E1",
		"1E-1",
		"1.2E3",
	}

	for _, test := range encodingTests {
		for _, sign := range []string{"", "+", "-"} {
			x := sign + test
			var tx Decimal
			tx.SetString(x)
			b, err := json.Marshal(&tx)
			if err != nil {
				t.Errorf("marshaling of %s failed: %s", &tx, err)
				continue
			}
			var rx Decimal
			if err := json.Unmarshal(b, &rx); err != nil {
				t.Errorf("unmarshaling of %s failed: %s", &tx, err)
				continue
			}
			if rx.CmpTotal(&tx) != 0 {
				t.Errorf("JSON encoding of %s failed: got %s want %s", &tx, &rx, &tx)
			}
		}
	}
}

// TestNewFromStringInvalid tests that NewFromString produces
// an error when parsing an invalid decimal.
func TestNewFromStringInvalid(t *testing.T) {
	tests := []struct {
		input string
	}{
		{input: ".-5"},
		{input: ".+5"},
		{input: ".-0"},
		{input: ".+0"},
		{input: ".-123"},
		{input: ".+123"},
		{input: "1.-2"},
		{input: "1.+2"},
		{input: ".1-2"},
		{input: ".1+2"},
		{input: ".-5e1"},
		{input: ".+5e1"},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			d, _, err := NewFromString(tc.input)
			if err == nil {
				t.Fatalf("expected error for %q, but parsed as: %s (Negative=%v, Coeff=%s)",
					tc.input, d.String(), d.Negative, d.Coeff.String())
			}
		})
	}
}

func TestRemUnlimitedPrecision(t *testing.T) {
	for _, tc := range []struct{ x, y, want string }{
		{"100E100", "10E1", "0E1"},
		{"123456789012345678901234567890", "7", "0"},
		{"123.45", "10", "3.45"},
		{"-123.45", "10", "-3.45"},
		{"123.45", "-10", "3.45"},
		{"0.01", "1", "0.01"},
		{"-10", "5", "-0"},
		{"-0", "3", "-0"},
	} {
		t.Run(tc.x+"/"+tc.y, func(t *testing.T) {
			for _, alias := range []string{"neither", "x", "y"} {
				t.Run(alias, func(t *testing.T) {
					x := newDecimal(t, &BaseContext, tc.x)
					y := newDecimal(t, &BaseContext, tc.y)
					want := newDecimal(t, &BaseContext, tc.want)
					d := new(Decimal)
					if alias == "x" {
						d = x
					}
					if alias == "y" {
						d = y
					}
					flags, err := BaseContext.Rem(d, x, y)
					if err != nil || flags != 0 {
						t.Fatalf("flags %s, error %v", flags, err)
					}
					if d.CmpTotal(want) != 0 {
						t.Fatalf("got %s, want %s", d, want)
					}
				})
			}
		})
	}
}

func TestRemLimitedPrecision(t *testing.T) {
	x, y := New(12345, 0), New(10, 0)
	var d Decimal
	flags, err := BaseContext.WithPrecision(3).Rem(&d, x, y)
	if flags != DivisionImpossible || err == nil || d.Form != NaN {
		t.Fatalf("got %s, flags %s, error %v", &d, flags, err)
	}
	flags, err = BaseContext.WithPrecision(4).Rem(&d, x, y)
	if flags != 0 || err != nil || d.Cmp(New(5, 0)) != 0 {
		t.Fatalf("got %s, flags %s, error %v", &d, flags, err)
	}
	x, y = New(100, 100), New(10, 1)
	flags, err = BaseContext.WithPrecision(1).Rem(&d, x, y)
	if flags != DivisionImpossible || err == nil || d.Form != NaN {
		t.Fatalf("got %s, flags %s, error %v", &d, flags, err)
	}
}
