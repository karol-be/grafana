package folderimpl

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana/pkg/infra/db"
	"github.com/grafana/grafana/pkg/services/folder"
	"github.com/grafana/grafana/pkg/services/sqlstore"
	"github.com/grafana/grafana/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var folders = [][]any{
	// org_id, uid, title, created, updated, parent_uid, left, right
	{1, "1", "ELECTRONICS", time.Now(), time.Now(), nil, 1, 20},
	{1, "2", "TELEVISIONS", time.Now(), time.Now(), "1", 2, 9},
	{1, "3", "TUBE", time.Now(), time.Now(), "2", 3, 4},
	{1, "4", "LCD", time.Now(), time.Now(), "2", 5, 6},
	{1, "5", "PLASMA", time.Now(), time.Now(), "2", 7, 8},
	{1, "6", "PORTABLE ELECTRONICS", time.Now(), time.Now(), "1", 10, 19},
	{1, "7", "MP3 PLAYERS", time.Now(), time.Now(), "6", 11, 14},
	{1, "8", "FLASH", time.Now(), time.Now(), "7", 12, 13},
	{1, "9", "CD PLAYERS", time.Now(), time.Now(), "6", 15, 16},
	{1, "10", "2 WAY RADIOS", time.Now(), time.Now(), "6", 17, 18},
}

// storeFolders stores the folders in the database.
// if storeLeftRight is true, the left and right values are stored as well.
func storeFolders(t *testing.T, storeDB db.DB, storeLeftRight bool) {
	t.Helper()

	storeDB.WithDbSession(context.Background(), func(sess *db.Session) error {
		cols := []string{"org_id", "uid", "title", "created", "updated", "parent_uid"}
		if storeLeftRight {
			cols = append(cols, "lft", "rgt")
		}
		sql := "INSERT INTO folder(" + strings.Join(cols, ",") + ") VALUES"
		sqlOrArgs := []any{}
		for i := 0; i < len(folders); i++ {
			sql = sql + "(" + strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"
			if i < len(folders)-1 {
				sql = sql + ","
			}
			sqlOrArgs = append(sqlOrArgs, folders[i][:len(cols)]...)
		}
		sqlOrArgs = append([]any{sql}, sqlOrArgs...)

		_, err := sess.Exec(sqlOrArgs...)
		require.NoError(t, err)
		return nil
	})

}

func TestIntegrationMigrate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	sqlStore := sqlstore.InitTestDB(t)

	folderStore := ProvideHierarchicalStore(sqlStore)
	storeFolders(t, folderStore.db, false)

	_, err := folderStore.migrate(context.Background(), 1, nil, 0)
	require.NoError(t, err)

	var r []*folder.Folder
	folderStore.db.WithDbSession(context.Background(), func(sess *db.Session) error {
		return sess.SQL("SELECT * FROM folder").Find(&r)
	})
	require.NoError(t, err)

	for i := 0; i < len(folders); i++ {
		assert.Equal(t, folders[i][0], int(r[i].OrgID))
		assert.Equal(t, folders[i][1], r[i].UID)
		assert.Equal(t, folders[i][6], int(r[i].Lft))
		assert.Equal(t, folders[i][7], int(r[i].Rgt))
	}

	tree, err := folderStore.getTree(context.Background(), 1)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"1-ELECTRONICS",
		"2-TELEVISIONS",
		"3-TUBE",
		"3-LCD",
		"3-PLASMA",
		"2-PORTABLE ELECTRONICS",
		"3-MP3 PLAYERS",
		"4-FLASH",
		"3-CD PLAYERS",
		"3-2 WAY RADIOS",
	}, tree)
}

func TestIntegrationGetParentsMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	sqlStore := sqlstore.InitTestDB(t)
	folderStore := ProvideHierarchicalStore(sqlStore)
	storeFolders(t, folderStore.db, true)

	ancestors, err := folderStore.GetParents(context.Background(), folder.GetParentsQuery{
		OrgID: 1,
		UID:   "8",
	})
	require.NoError(t, err)

	expected := []string{"ELECTRONICS", "PORTABLE ELECTRONICS", "MP3 PLAYERS"}
	assert.Equal(t, len(expected), len(ancestors))
	for i := 0; i < len(ancestors); i++ {
		assert.Equal(t, expected[i], ancestors[i].Title)
	}
}

func TestIntegrationCreateMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testCases := []struct {
		desc         string
		expectedTree []string
		parentUID    string
	}{
		{
			desc:      "create folder under root",
			parentUID: "",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
				"1-NEW FOLDER",
			},
		},
		{
			desc:      "create folder under TELEVISIONS",
			parentUID: "2",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"3-NEW FOLDER",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:      "create folder under TUBE",
			parentUID: "3",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"4-NEW FOLDER",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:      "create folder under FLASH",
			parentUID: "8",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"5-NEW FOLDER",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:      "create folder under ELECTRONICS",
			parentUID: "1",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
				"2-NEW FOLDER",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			sqlStore := sqlstore.InitTestDB(t)
			folderStore := ProvideHierarchicalStore(sqlStore)
			storeFolders(t, folderStore.db, true)

			_, err := folderStore.Create(context.Background(), folder.CreateFolderCommand{
				OrgID:     1,
				UID:       "22",
				Title:     "NEW FOLDER",
				ParentUID: tc.parentUID,
			})
			require.NoError(t, err)

			tree, err := folderStore.getTree(context.Background(), 1)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedTree, tree)
		})
	}
}

func TestIntegrationGetChildrenMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	sqlStore := sqlstore.InitTestDB(t)
	folderStore := ProvideHierarchicalStore(sqlStore)
	storeFolders(t, folderStore.db, true)

	testCases := []struct {
		desc             string
		uid              string
		expectedChildren []string
	}{
		{
			desc:             "get children of ELECTRONICS",
			uid:              "1",
			expectedChildren: []string{"PORTABLE ELECTRONICS", "TELEVISIONS"},
		},
		{
			desc:             "get children of TELEVISIONS",
			uid:              "2",
			expectedChildren: []string{"LCD", "PLASMA", "TUBE"},
		},
		{
			desc:             "get children of TUBE",
			uid:              "3",
			expectedChildren: []string{},
		},
		{
			desc:             "get children of LCD",
			uid:              "4",
			expectedChildren: []string{},
		},
		{
			desc:             "get children of PLASMA",
			uid:              "5",
			expectedChildren: []string{},
		},
		{
			desc:             "get children of PORTABLE ELECTRONICS",
			uid:              "6",
			expectedChildren: []string{"2 WAY RADIOS", "CD PLAYERS", "MP3 PLAYERS"},
		},
		{
			desc:             "get children of MP3 PLAYERS",
			uid:              "7",
			expectedChildren: []string{"FLASH"},
		},
		{
			desc:             "get children of FLASH",
			uid:              "8",
			expectedChildren: []string{},
		},
		{
			desc:             "get children of CD PLAYERS",
			uid:              "9",
			expectedChildren: []string{},
		},
		{
			desc:             "get children of 2 WAY RADIOS",
			uid:              "10",
			expectedChildren: []string{},
		},
	}

	for _, tc := range testCases {
		children, err := folderStore.GetChildren(context.Background(), folder.GetChildrenQuery{
			OrgID: 1,
			UID:   tc.uid,
		})
		require.NoError(t, err)

		assert.Equal(t, len(tc.expectedChildren), len(children))
		for i := 0; i < len(children); i++ {
			assert.Equal(t, tc.expectedChildren[i], children[i].Title)
		}
	}
}

func TestIntegrationGetHeightMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testCases := []struct {
		desc     string
		uid      string
		expected int
	}{
		{
			desc:     "get height of ELECTRONICS",
			uid:      "1",
			expected: 3,
		},
		{
			desc:     "get height of TELEVISIONS",
			uid:      "2",
			expected: 1,
		},
		{
			desc:     "get height of TUBE",
			uid:      "3",
			expected: 0,
		},
		{
			desc:     "get height of LCD",
			uid:      "4",
			expected: 0,
		},
		{
			desc:     "get height of PLASMA",
			uid:      "5",
			expected: 0,
		},
		{
			desc:     "get height of PORTABLE ELECTRONICS",
			uid:      "6",
			expected: 2,
		},
		{
			desc:     "get height of MP3 PLAYERS",
			uid:      "7",
			expected: 1,
		},
		{
			desc:     "get height of FLASH",
			uid:      "8",
			expected: 0,
		},
		{
			desc:     "get height of CD PLAYERS",
			uid:      "9",
			expected: 0,
		},
		{
			desc:     "get height of 2 WAY RADIOS",
			uid:      "10",
			expected: 0,
		},
	}

	sqlStore := sqlstore.InitTestDB(t)
	folderStore := ProvideHierarchicalStore(sqlStore)
	storeFolders(t, folderStore.db, true)

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			height, err := folderStore.GetHeight(context.Background(), tc.uid, 1, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, height)
		})
	}
}

func TestIntegrationUpdateMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	sqlStore := sqlstore.InitTestDB(t)
	folderStore := ProvideHierarchicalStore(sqlStore)
	storeFolders(t, folderStore.db, true)

	testCases := []struct {
		desc  string
		UID   *string
		ID    *int64
		Title *string
	}{
		{
			desc: "get by uid",
			UID:  util.Pointer("8"),
		},
		{
			desc: "get by id",
			ID:   util.Pointer(int64(8)),
		},
		{
			desc:  "get by title",
			Title: util.Pointer("FLASH"),
		},
	}

	for _, tc := range testCases {
		f, err := folderStore.Get(context.Background(), folder.GetFolderQuery{
			OrgID: 1,
			UID:   tc.UID,
			ID:    tc.ID,
			Title: tc.Title,
		})
		require.NoError(t, err)

		assert.Equal(t, "FLASH", f.Title)
		assert.Equal(t, "8", f.UID)
		assert.Equal(t, int64(8), f.ID)
		//assert.NotEmpty(t, f.FullPathUIDs)
		//assert.NotEmpty(t, f.FullPath)
	}
}

func TestIntegrationDeleteMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testCases := []struct {
		desc         string
		UID          string
		expectedTree []string
	}{
		{
			desc:         "delete folder under root",
			UID:          "1",
			expectedTree: nil,
		},
		{
			desc: "delete TELEVISIONS",
			UID:  "2",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete TUBE",
			UID:  "3",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete LCD",
			UID:  "4",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete PLASMA",
			UID:  "5",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete PORTABLE ELECTRONICS",
			UID:  "6",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
			},
		},
		{
			desc: "delete MP3 PLAYERS",
			UID:  "7",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete FLASH",
			UID:  "8",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete CD PLAYERS",
			UID:  "9",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc: "delete 2 WAY RADIOS",
			UID:  "10",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			sqlStore := sqlstore.InitTestDB(t)
			folderStore := ProvideHierarchicalStore(sqlStore)
			storeFolders(t, folderStore.db, true)

			err := folderStore.Delete(context.Background(), tc.UID, 1)
			require.NoError(t, err)

			tree, err := folderStore.getTree(context.Background(), 1)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedTree, tree)
		})
	}
}

func TestIntegrationMoveMPTT(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testCases := []struct {
		desc         string
		UID          string
		newParentUID string
		expectedTree []string
	}{
		{
			desc:         "move TELEVISIONS under FLASH",
			UID:          "2",
			newParentUID: "8",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"5-TELEVISIONS",
				"6-TUBE",
				"6-LCD",
				"6-PLASMA",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:         "move TELEVISIONS under MP3 PLAYERS",
			UID:          "2",
			newParentUID: "7",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-TELEVISIONS",
				"5-TUBE",
				"5-LCD",
				"5-PLASMA",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:         "move TELEVISIONS under CD PLAYERS",
			UID:          "2",
			newParentUID: "9",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"3-CD PLAYERS",
				"4-TELEVISIONS",
				"5-TUBE",
				"5-LCD",
				"5-PLASMA",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:         "move TUBE under MP3 PLAYERS",
			UID:          "3",
			newParentUID: "7",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-TUBE",
				"4-FLASH",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:         "move TUBE under FLASH",
			UID:          "3",
			newParentUID: "8",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-LCD",
				"3-PLASMA",
				"2-PORTABLE ELECTRONICS",
				"3-MP3 PLAYERS",
				"4-FLASH",
				"5-TUBE",
				"3-CD PLAYERS",
				"3-2 WAY RADIOS",
			},
		},
		{
			desc:         "move PORTABLE ELECTRONICS under TUBE",
			UID:          "6",
			newParentUID: "3",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"4-PORTABLE ELECTRONICS",
				"5-MP3 PLAYERS",
				"6-FLASH",
				"5-CD PLAYERS",
				"5-2 WAY RADIOS",
				"3-LCD",
				"3-PLASMA",
			},
		},
		{
			desc:         "move PORTABLE ELECTRONICS under TELEVISIONS",
			UID:          "6",
			newParentUID: "2",
			expectedTree: []string{
				"1-ELECTRONICS",
				"2-TELEVISIONS",
				"3-TUBE",
				"3-LCD",
				"3-PLASMA",
				"3-PORTABLE ELECTRONICS",
				"4-MP3 PLAYERS",
				"5-FLASH",
				"4-CD PLAYERS",
				"4-2 WAY RADIOS",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			sqlStore := sqlstore.InitTestDB(t)
			folderStore := ProvideHierarchicalStore(sqlStore)
			storeFolders(t, folderStore.db, true)

			_, err := folderStore.Update(context.Background(), folder.UpdateFolderCommand{
				OrgID:        1,
				UID:          tc.UID,
				NewParentUID: &tc.newParentUID,
			})
			require.NoError(t, err)
			tree, err := folderStore.getTree(context.Background(), 1)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedTree, tree)
		})
	}
}
