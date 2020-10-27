/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package common

import (
	"fmt"

	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// DecodeOpaqueMap decodes a Reva opaque object into a map of strings.
// Only plain decoders are currently supported.
func DecodeOpaqueMap(opaque *types.Opaque) map[string]string {
	entries := make(map[string]string)

	if opaque != nil {
		for k, v := range opaque.GetMap() {
			// Only plain values are currently supported
			switch v.Decoder {
			case "plain":
				entries[k] = string(v.Value)
			}
		}
	}

	return entries
}

// GetValuesFromOpaque extracts the given keys from the opaque object.
// If mandatory is set to true, all specified keys must be available in the opaque object.
func GetValuesFromOpaque(opaque *types.Opaque, keys []string, mandatory bool) (map[string]string, error) {
	values := make(map[string]string)
	entries := DecodeOpaqueMap(opaque)

	for _, key := range keys {
		if value, ok := entries[key]; ok {
			values[key] = value
		} else {
			if mandatory {
				return map[string]string{}, fmt.Errorf("missing opaque entry '%v'", key)
			}
		}
	}

	return values, nil
}
