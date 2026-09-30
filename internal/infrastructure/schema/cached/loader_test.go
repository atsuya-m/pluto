package cached_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/atsuya-m/pluto/internal/infrastructure/schema/cached"
	"github.com/atsuya-m/pluto/internal/testutil/fixture"
)

func TestSchemaLoader_LoadsOnce(t *testing.T) {
	inner := fixture.NewSchemaLoader(t)
	loader := cached.NewSchemaLoader(inner)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s, err := loader.Load(context.Background()); err != nil || s == nil {
				t.Errorf("Load = %v, %v", s, err)
			}
		}()
	}
	wg.Wait()
	if inner.Calls != 1 {
		t.Errorf("inner loader called %d times, want 1", inner.Calls)
	}
}

func TestSchemaLoader_CachesError(t *testing.T) {
	inner := &fixture.SchemaLoader{Err: errors.New("boom")}
	loader := cached.NewSchemaLoader(inner)
	for range 2 {
		if _, err := loader.Load(context.Background()); err == nil {
			t.Error("expected error")
		}
	}
	if inner.Calls != 1 {
		t.Errorf("inner loader called %d times, want 1", inner.Calls)
	}
}
