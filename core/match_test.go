package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCast(t *testing.T) {
	t.Run("Nested Map Slices", func(t *testing.T) {
		got := cast([]Map{
			{
				"another map slice": []Map{
					{
						"foo": "bar",
					},
				},
			},
		})
		assert.Equal(t,
			[]interface{}{
				map[string]interface{}{
					"another map slice": []interface{}{
						map[string]interface{}{
							"foo": "bar",
						},
					},
				},
			},
			got,
		)
	})
}
