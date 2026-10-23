package kopia

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareStoreAllowsEmptyPrefix(t *testing.T) {
	store := &fakeObjectStore{}

	err := prepareStore(context.Background(), store, testRepository())

	require.NoError(t, err)
	require.Empty(t, store.removed)
}

func TestPrepareStoreRemovesEmptyPrefixMarker(t *testing.T) {
	store := &fakeObjectStore{
		objects: []storedObject{{key: RepositoryPrefix, size: 0, hasSize: true}},
	}

	err := prepareStore(context.Background(), store, testRepository())

	require.NoError(t, err)
	require.Equal(t, []string{RepositoryPrefix}, store.removed)
}

func TestPrepareStoreRejectsExistingData(t *testing.T) {
	tests := map[string]struct {
		objects   []storedObject
		truncated bool
	}{
		"non-empty prefix object": {
			objects: []storedObject{{key: RepositoryPrefix, size: 1, hasSize: true}},
		},
		"object without size": {
			objects: []storedObject{{key: RepositoryPrefix}},
		},
		"different object": {
			objects: []storedObject{{key: RepositoryPrefix + "unknown", hasSize: true}},
		},
		"marker and another object": {
			objects: []storedObject{
				{key: RepositoryPrefix, hasSize: true},
				{key: RepositoryPrefix + "unknown", hasSize: true},
			},
		},
		"truncated listing": {
			objects:   []storedObject{{key: RepositoryPrefix, hasSize: true}},
			truncated: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := &fakeObjectStore{
				objects:   test.objects,
				truncated: test.truncated,
			}

			err := prepareStore(context.Background(), store, testRepository())

			require.ErrorContains(t, err, "contains existing data")
			require.Empty(t, store.removed)
		})
	}
}

func TestPrepareStoreReturnsStorageErrors(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		store := &fakeObjectStore{listErr: errors.New("list failed")}

		err := prepareStore(context.Background(), store, testRepository())

		require.ErrorContains(t, err, "list failed")
	})

	t.Run("remove", func(t *testing.T) {
		store := &fakeObjectStore{
			objects:   []storedObject{{key: RepositoryPrefix, hasSize: true}},
			removeErr: errors.New("remove failed"),
		}

		err := prepareStore(context.Background(), store, testRepository())

		require.ErrorContains(t, err, "remove failed")
	})
}

func testRepository() *Repository {
	return &Repository{Bucket: "bucket", Prefix: RepositoryPrefix}
}

type fakeObjectStore struct {
	objects   []storedObject
	truncated bool
	listErr   error
	removeErr error
	removed   []string
}

func (s *fakeObjectStore) list(
	context.Context,
	string,
	string,
	int64,
) ([]storedObject, bool, error) {
	return s.objects, s.truncated, s.listErr
}

func (s *fakeObjectStore) remove(_ context.Context, _, key string) error {
	if s.removeErr != nil {
		return s.removeErr
	}
	s.removed = append(s.removed, key)
	return nil
}
