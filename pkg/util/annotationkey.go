package util

func GenerateAnnotationKeyMigratingVMName(vmName string) string {
	return AnnotationMigratingNamePrefix + vmName
}

func GenerateAnnotationKeyMigratingVMUID(vmUID string) string {
	return AnnotationMigratingUIDPrefix + vmUID
}
