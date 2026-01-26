package util

func GenerateAnnotationKeyMigratingVMUID(vmUID string) string {
	return AnnotationMigratingUIDPrefix + vmUID
}
