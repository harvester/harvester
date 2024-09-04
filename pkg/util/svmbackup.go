package util

import (
	"math"
	"time"

	"github.com/robfig/cron"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	ctlharvesterv1 "github.com/harvester/harvester/pkg/generated/controllers/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/ref"
)

func ResolveSVMBackupRef(svmbackupCache ctlharvesterv1.ScheduleVMBackupCache, obj metav1.Object) *harvesterv1.ScheduleVMBackup {
	var annotations = obj.GetAnnotations()
	if annotations == nil || annotations[AnnotationSVMBackupID] == "" {
		return nil
	}

	namespace, name := ref.Parse(annotations[AnnotationSVMBackupID])
	svmbackup, err := svmbackupCache.Get(namespace, name)
	if err != nil {
		return nil
	}

	return svmbackup
}

func GetCronGranularity(svmbackup *harvesterv1.ScheduleVMBackup) (time.Duration, error) {
	schedule, err := cron.ParseStandard(svmbackup.Spec.Cron)
	if err != nil {
		return time.Duration(math.MinInt64), err
	}

	nextTime := schedule.Next(time.Now())
	followingTime := schedule.Next(nextTime)
	return followingTime.Sub(nextTime), nil
}

func CalculateCronOffset(svmbackup1, svmbackup2 *harvesterv1.ScheduleVMBackup) (time.Duration, error) {
	schedule1, err := cron.ParseStandard(svmbackup1.Spec.Cron)
	if err != nil {
		return time.Duration(math.MinInt64), err
	}

	schedule2, err := cron.ParseStandard(svmbackup2.Spec.Cron)
	if err != nil {
		return time.Duration(math.MinInt64), err
	}

	currentTime := time.Now()

	// Get the second next run times from the current time for both schedules
	nextRun1 := schedule1.Next(schedule1.Next(currentTime))
	nextRun2 := schedule2.Next(schedule2.Next(currentTime))

	// Calculate the offset in minutes
	offset := nextRun2.Sub(nextRun1)
	if offset >= 0 {
		return offset, nil
	}

	return -offset, nil
}

func SkipCronGranularityCheck(svmbackup *harvesterv1.ScheduleVMBackup) bool {
	var annotations = svmbackup.GetAnnotations()
	if annotations == nil || annotations[AnnotationSVMBackupSkipCronCheck] == "" {
		return false
	}

	return true
}
