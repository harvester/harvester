package backupstore

const (
	DEFAULT_BLOCK_SIZE        = 2 * 1024 * 1024
	LEGACY_COMPRESSION_METHOD = "gzip"

	BLOCKS_DIRECTORY      = "blocks"
	BLOCK_SEPARATE_LAYER1 = 2
	BLOCK_SEPARATE_LAYER2 = 4
	BLK_SUFFIX            = ".blk"

	PROGRESS_PERCENTAGE_BACKUP_SNAPSHOT = 95
	PROGRESS_PERCENTAGE_BACKUP_TOTAL    = 100
)

type DataEngine string

const (
	DataEngineV1 = DataEngine("v1")
	DataEngineV2 = DataEngine("v2")
)
