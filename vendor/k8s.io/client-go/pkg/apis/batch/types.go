/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package batch

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api"
)

// +genclient=true

// Job represents the configuration of a single job.
type Job struct {
	metav1.TypeMeta
	// Standard object's metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta

	// Spec is a structure defining the expected behavior of a job.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	// +optional
	Spec JobSpec

	// Status is a structure describing current status of a job.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	// +optional
	Status JobStatus
}

// JobList is a collection of jobs.
type JobList struct {
	metav1.TypeMeta
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	// +optional
	metav1.ListMeta

	// Items is the list of Job.
	Items []Job
}

// JobTemplate describes a template for creating copies of a predefined pod.
type JobTemplate struct {
	metav1.TypeMeta
	// Standard object's metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta

	// Template defines jobs that will be created from this template
	// http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	// +optional
	Template JobTemplateSpec
}

// JobTemplateSpec describes the data a Job should have when created from a template
type JobTemplateSpec struct {
	// Standard object's metadata of the jobs created from this template.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta

	// Specification of the desired behavior of the job.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	// +optional
	Spec JobSpec
}

// JobSpec describes how the job execution will look like.
type JobSpec struct {

	// Parallelism specifies the maximum desired number of pods the job should
	// run at any given time. The actual number of pods running in steady state will
	// be less than this number when ((.spec.completions - .status.successful) < .spec.parallelism),
	// i.e. when the work left to do is less than max parallelism.
	// +optional
	Parallelism *int32

	// Completions specifies the desired number of successfully finished pods the
	// job should be run with.  Setting to nil means that the success of any
	// pod signals the success of all pods, and allows parallelism to have any positive
	// value.  Setting to 1 means that parallelism is limited to 1 and the success of that
	// pod signals the success of the job.
	// +optional
	Completions *int32

	// Optional duration in seconds relative to the startTime that the job may be active
	// before the system tries to terminate it; value must be positive integer
	// +optional
	ActiveDeadlineSeconds *int64

	// Selector is a label query over pods that should match the pod count.
	// Normally, the system sets this field for you.
	// +optional
	Selector *metav1.LabelSelector

	// ManualSelector controls generation of pod labels and pod selectors.
	// Leave `manualSelector` unset unless you are certain what you are doing.
	// When false or unset, the system pick labels unique to this job
	// and appends those labels to the pod template.  When true,
	// the user is responsible for picking unique labels and specifying
	// the selector.  Failure to pick a unique label may cause this
	// and other jobs to not function correctly.  However, You may see
	// `manualSelector=true` in jobs that were created with the old `extensions/v1beta1`
	// API.
	// +optional
	ManualSelector *bool

	// Template is the object that describes the pod that will be created when
	// executing a job.
	Template api.PodTemplateSpec
}

// JobStatus represents the current state of a Job.
type JobStatus struct {

	// Conditions represent the latest available observations of an object's current state.
	// +optional
	Conditions []JobCondition

	// StartTime represents time when the job was acknowledged by the Job Manager.
	// It is not guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	StartTime *metav1.Time

	// CompletionTime represents time when the job was completed. It is not guaranteed to
	// be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	// +optional
	CompletionTime *metav1.Time

	// Active is the number of actively running pods.
	// +optional
	Active int32

	// Succeeded is the number of pods which reached Phase Succeeded.
	// +optional
	Succeeded int32

	// Failed is the number of pods which reached Phase Failed.
	// +optional
	Failed int32
}

type JobConditionType string

// These are valid conditions of a job.
const (
	// JobComplete means the job has completed its execution.
	JobComplete JobConditionType = "Complete"
	// JobFailed means the job has failed its execution.
	JobFailed JobConditionType = "Failed"
)

// JobCondition describes current state of a job.
type JobCondition struct {
	// Type of job condition, Complete or Failed.
	Type JobConditionType
	// Status of the condition, one of True, False, Unknown.
	Status api.ConditionStatus
	// Last time the condition was checked.
	// +optional
	LastProbeTime metav1.Time
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string
	// Human readable message indicating details about last transition.
	// +optional
	Message string
}

// +genclient=true

// CronJob represents the configuration of a single cron job.
type CronJob struct {
	metav1.TypeMeta
	// Standard object's metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta

	// Spec is a structure defining the expected behavior of a job, including the schedule.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	// +optional
	Spec CronJobSpec

	// Status is a structure describing current status of a job.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	// +optional
	Status CronJobStatus
}

// CronJobList is a collection of cron jobs.
type CronJobList struct {
	metav1.TypeMeta
	// Standard list metadata
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	// +optional
	metav1.ListMeta

	// Items is the list of CronJob.
	Items []CronJob
}

// CronJobSpec describes how the job execution will look like and when it will actually run.
type CronJobSpec struct {

	// Schedule contains the schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string

	// Optional deadline in seconds for starting the job if it misses scheduled
	// time for any reason.  Missed jobs executions will be counted as failed ones.
	// +optional
	StartingDeadlineSeconds *int64

	// ConcurrencyPolicy specifies how to treat concurrent executions of a Job.
	// +optional
	ConcurrencyPolicy ConcurrencyPolicy

	// Suspend flag tells the controller to suspend subsequent executions, it does
	// not apply to already started executions.  Defaults to false.
	// +optional
	Suspend *bool

	// JobTemplate is the object that describes the job that will be created when
	// executing a CronJob.
	JobTemplate JobTemplateSpec

	// The number of successful finished jobs to retain.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	SuccessfulJobsHistoryLimit *int32

	// The number of failed finished jobs to retain.
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	FailedJobsHistoryLimit *int32
}

// ConcurrencyPolicy describes how the job will be handled.
// Only one of the following concurrent policies may be specified.
// If none of the following policies is specified, the default one
// is AllowConcurrent.
type ConcurrencyPolicy string

const (
	// AllowConcurrent allows CronJobs to run concurrently.
	AllowConcurrent ConcurrencyPolicy = "Allow"

	// ForbidConcurrent forbids concurrent runs, skipping next run if previous
	// hasn't finished yet.
	ForbidConcurrent ConcurrencyPolicy = "Forbid"

	// ReplaceConcurrent cancels currently running job and replaces it with a new one.
	ReplaceConcurrent ConcurrencyPolicy = "Replace"
)

// CronJobStatus represents the current state of a cron job.
type CronJobStatus struct {
	// Active holds pointers to currently running jobs.
	// +optional
	Active []api.ObjectReference

	// LastScheduleTime keeps information of when was the last time the job was successfully scheduled.
	// +optional
	LastScheduleTime *metav1.Time
}
