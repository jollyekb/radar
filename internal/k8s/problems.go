package k8s

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Problem is a transport-neutral cluster issue.
type Problem struct {
	Kind            string
	Namespace       string
	Name            string
	Severity        string // "critical", "high", or "medium"
	Reason          string
	Message         string
	Age             string // human-readable
	AgeSeconds      int64  // for sorting
	Duration        string // how long the problem has persisted
	DurationSeconds int64
}

// DetectProblems scans workloads in cache and returns detected problems.
// Covers: Deployments, StatefulSets, DaemonSets, HPAs, CronJobs, Nodes.
// Does NOT include pods (consumers handle pod problems differently).
// namespace="" scans all namespaces.
func DetectProblems(cache *ResourceCache, namespace string) []Problem {
	var problems []Problem
	now := time.Now()

	// Deployment problems: unavailableReplicas > 0
	if depLister := cache.Deployments(); depLister != nil {
		var deps []*appsv1.Deployment
		if namespace != "" {
			deps, _ = depLister.Deployments(namespace).List(labels.Everything())
		} else {
			deps, _ = depLister.List(labels.Everything())
		}
		for _, d := range deps {
			if d.Status.UnavailableReplicas > 0 {
				ageDur := now.Sub(d.CreationTimestamp.Time)
				durDur := ageDur // fallback to creation time
				for _, cond := range d.Status.Conditions {
					if cond.Type == appsv1.DeploymentAvailable && cond.Status == "False" && !cond.LastTransitionTime.IsZero() {
						durDur = now.Sub(cond.LastTransitionTime.Time)
						break
					}
				}
				problems = append(problems, Problem{
					Kind:            "Deployment",
					Namespace:       d.Namespace,
					Name:            d.Name,
					Severity:        "critical",
					Reason:          fmt.Sprintf("%d/%d available", d.Status.AvailableReplicas, d.Status.Replicas),
					Age:             FormatAge(ageDur),
					AgeSeconds:      int64(ageDur.Seconds()),
					Duration:        FormatAge(durDur),
					DurationSeconds: int64(durDur.Seconds()),
				})
			}
			// Stuck rollout: ProgressDeadlineExceeded
			for _, cond := range d.Status.Conditions {
				if cond.Type == appsv1.DeploymentProgressing && cond.Status == "False" && cond.Reason == "ProgressDeadlineExceeded" {
					durDur := now.Sub(d.CreationTimestamp.Time)
					if !cond.LastTransitionTime.IsZero() {
						durDur = now.Sub(cond.LastTransitionTime.Time)
					}
					problems = append(problems, Problem{
						Kind:            "Deployment",
						Namespace:       d.Namespace,
						Name:            d.Name,
						Severity:        "critical",
						Reason:          "Rollout stuck",
						Message:         cond.Message,
						Age:             FormatAge(now.Sub(d.CreationTimestamp.Time)),
						AgeSeconds:      int64(now.Sub(d.CreationTimestamp.Time).Seconds()),
						Duration:        FormatAge(durDur),
						DurationSeconds: int64(durDur.Seconds()),
					})
					break
				}
			}
		}
	}

	// StatefulSet problems: readyReplicas < replicas
	if ssLister := cache.StatefulSets(); ssLister != nil {
		var ssets []*appsv1.StatefulSet
		if namespace != "" {
			ssets, _ = ssLister.StatefulSets(namespace).List(labels.Everything())
		} else {
			ssets, _ = ssLister.List(labels.Everything())
		}
		for _, ss := range ssets {
			if ss.Status.ReadyReplicas < ss.Status.Replicas {
				ageDur := now.Sub(ss.CreationTimestamp.Time)
				problems = append(problems, Problem{
					Kind:            "StatefulSet",
					Namespace:       ss.Namespace,
					Name:            ss.Name,
					Severity:        "critical",
					Reason:          fmt.Sprintf("%d/%d ready", ss.Status.ReadyReplicas, ss.Status.Replicas),
					Age:             FormatAge(ageDur),
					AgeSeconds:      int64(ageDur.Seconds()),
					Duration:        FormatAge(ageDur),
					DurationSeconds: int64(ageDur.Seconds()),
				})
			}
		}
	}

	// DaemonSet problems: numberUnavailable > 0
	if dsLister := cache.DaemonSets(); dsLister != nil {
		var dsets []*appsv1.DaemonSet
		if namespace != "" {
			dsets, _ = dsLister.DaemonSets(namespace).List(labels.Everything())
		} else {
			dsets, _ = dsLister.List(labels.Everything())
		}
		for _, ds := range dsets {
			if ds.Status.NumberUnavailable > 0 {
				ageDur := now.Sub(ds.CreationTimestamp.Time)
				problems = append(problems, Problem{
					Kind:            "DaemonSet",
					Namespace:       ds.Namespace,
					Name:            ds.Name,
					Severity:        "critical",
					Reason:          fmt.Sprintf("%d unavailable", ds.Status.NumberUnavailable),
					Age:             FormatAge(ageDur),
					AgeSeconds:      int64(ageDur.Seconds()),
					Duration:        FormatAge(ageDur),
					DurationSeconds: int64(ageDur.Seconds()),
				})
			}
		}
	}

	// HPA problems
	if hpaLister := cache.HorizontalPodAutoscalers(); hpaLister != nil {
		var hpas []*autoscalingv2.HorizontalPodAutoscaler
		if namespace != "" {
			hpas, _ = hpaLister.HorizontalPodAutoscalers(namespace).List(labels.Everything())
		} else {
			hpas, _ = hpaLister.List(labels.Everything())
		}
		for _, hp := range DetectHPAProblems(hpas) {
			problems = append(problems, Problem{
				Kind:      "HorizontalPodAutoscaler",
				Namespace: hp.Namespace,
				Name:      hp.Name,
				Severity:  "medium",
				Reason:    hp.Problem,
				Message:   hp.Reason,
			})
		}
	}

	// CronJob problems
	if cjLister := cache.CronJobs(); cjLister != nil {
		var cronjobs []*batchv1.CronJob
		if namespace != "" {
			cronjobs, _ = cjLister.CronJobs(namespace).List(labels.Everything())
		} else {
			cronjobs, _ = cjLister.List(labels.Everything())
		}
		for _, cp := range DetectCronJobProblems(cronjobs) {
			problems = append(problems, Problem{
				Kind:      "CronJob",
				Namespace: cp.Namespace,
				Name:      cp.Name,
				Severity:  "medium",
				Reason:    cp.Problem,
				Message:   cp.Reason,
			})
		}
	}

	// Node problems (cluster-scoped, not filtered by namespace)
	if nodeLister := cache.Nodes(); nodeLister != nil {
		nodes, _ := nodeLister.List(labels.Everything())
		for _, np := range DetectNodeProblems(nodes) {
			ageDur := time.Duration(0)
			for _, n := range nodes {
				if n.Name == np.NodeName {
					ageDur = now.Sub(n.CreationTimestamp.Time)
					break
				}
			}
			problems = append(problems, Problem{
				Kind:       "Node",
				Name:       np.NodeName,
				Severity:   np.Severity,
				Reason:     np.Problem,
				Message:    np.Reason,
				Age:        FormatAge(ageDur),
				AgeSeconds: int64(ageDur.Seconds()),
			})
		}
	}

	// PVC problems: stuck in Pending phase
	if pvcLister := cache.PersistentVolumeClaims(); pvcLister != nil {
		var pvcs []*corev1.PersistentVolumeClaim
		if namespace != "" {
			pvcs, _ = pvcLister.PersistentVolumeClaims(namespace).List(labels.Everything())
		} else {
			pvcs, _ = pvcLister.List(labels.Everything())
		}
		for _, pvc := range pvcs {
			if pvc.Status.Phase == corev1.ClaimPending {
				ageDur := now.Sub(pvc.CreationTimestamp.Time)
				if ageDur > 5*time.Minute {
					problems = append(problems, Problem{
						Kind:            "PersistentVolumeClaim",
						Namespace:       pvc.Namespace,
						Name:            pvc.Name,
						Severity:        "high",
						Reason:          "Pending",
						Age:             FormatAge(ageDur),
						AgeSeconds:      int64(ageDur.Seconds()),
						Duration:        FormatAge(ageDur),
						DurationSeconds: int64(ageDur.Seconds()),
					})
				}
			}
		}
	}

	// Job problems: stuck active (running > 1h with no completions)
	if jobLister := cache.Jobs(); jobLister != nil {
		var jobs []*batchv1.Job
		if namespace != "" {
			jobs, _ = jobLister.Jobs(namespace).List(labels.Everything())
		} else {
			jobs, _ = jobLister.List(labels.Everything())
		}
		for _, job := range jobs {
			if job.Status.Active > 0 && job.Status.Succeeded == 0 && job.Status.Failed == 0 {
				ageDur := now.Sub(job.CreationTimestamp.Time)
				if ageDur > time.Hour {
					problems = append(problems, Problem{
						Kind:            "Job",
						Namespace:       job.Namespace,
						Name:            job.Name,
						Severity:        "high",
						Reason:          fmt.Sprintf("Running for %s with no completions", FormatAge(ageDur)),
						Age:             FormatAge(ageDur),
						AgeSeconds:      int64(ageDur.Seconds()),
						Duration:        FormatAge(ageDur),
						DurationSeconds: int64(ageDur.Seconds()),
					})
				}
			}
		}
	}

	return problems
}
