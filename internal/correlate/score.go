package correlate

import (
	"fmt"
	"strings"

	"github.com/Yatsuiii/costtrace/internal/storage"
)

type Evidence struct {
	Kind        string
	Description string
	ResourceIDs []string
}

func (e Evidence) String() string {
	s := fmt.Sprintf("[%s] %s", e.Kind, e.Description)
	if len(e.ResourceIDs) > 0 {
		s += fmt.Sprintf(" (%s)", strings.Join(e.ResourceIDs, ", "))
	}
	return s
}

// scoreWithLineage augments time-window candidates with CloudTrail evidence.
// ctByDeploy maps deployID → cloudtrail events for that deploy.
// costRows is the full cost dataset for overlap detection (service-level).
func scoreWithLineage(
	candidates []Candidate,
	ctByDeploy map[string][]storage.CloudTrailRow,
	anomalyService string,
) []Candidate {
	// map service name keywords → aws resource types that drive cost
	serviceKeywords := serviceToResourceHints(anomalyService)

	var scored []Candidate
	for _, c := range candidates {
		events := ctByDeploy[c.Deploy.ID]
		if len(events) == 0 {
			scored = append(scored, c)
			continue
		}

		conf := 0.05 // time-window base
		var evidences []Evidence

		// resource creation in deploy window
		var matchedResources []string
		for _, e := range events {
			if matchesService(e.ResourceType, serviceKeywords) {
				matchedResources = append(matchedResources, e.ResourceID)
			}
		}
		if len(matchedResources) > 0 {
			conf += 0.6
			evidences = append(evidences, Evidence{
				Kind:        "resource_creation",
				Description: fmt.Sprintf("%d resource(s) created matching service %q", len(matchedResources), anomalyService),
				ResourceIDs: dedup(matchedResources),
			})
		}

		// principal match: CI/CD user agents
		for _, e := range events {
			if isCIAgent(e.UserAgent) || isCIAgent(e.PrincipalID) {
				conf += 0.2
				evidences = append(evidences, Evidence{
					Kind:        "principal_match",
					Description: fmt.Sprintf("CloudTrail principal %q looks like CI/CD", e.PrincipalID),
				})
				break
			}
		}

		if conf > 1.0 {
			conf = 1.0
		}

		allEvidence := c.Evidence
		for _, ev := range evidences {
			allEvidence += "\n      " + ev.String()
		}
		scored = append(scored, Candidate{
			Deploy:     c.Deploy,
			Confidence: conf,
			Evidence:   allEvidence,
		})
	}
	return scored
}

// serviceToResourceHints maps a Cost Explorer service name to lowercase
// substrings that should appear in CloudTrail resource types created by that
// service. Cost Explorer reports services in their canonical long form (e.g.
// "Amazon Elastic Compute Cloud - Compute"), so each case has to match both
// the short tag (ec2) and the long form (elastic compute).
func serviceToResourceHints(service string) []string {
	s := strings.ToLower(service)
	switch {
	case strings.Contains(s, "ec2") || strings.Contains(s, "elastic compute"):
		return []string{"instance", "volume", "aws::ec2"}
	case strings.Contains(s, "s3") || strings.Contains(s, "simple storage"):
		return []string{"bucket", "aws::s3"}
	case strings.Contains(s, "rds") || strings.Contains(s, "relational database"):
		return []string{"dbinstance", "dbcluster", "aws::rds"}
	case strings.Contains(s, "lambda"):
		return []string{"function", "aws::lambda"}
	case strings.Contains(s, "elb") || strings.Contains(s, "elastic load"):
		return []string{"loadbalancer", "aws::elasticloadbalancing"}
	case strings.Contains(s, "elasticache"):
		return []string{"cachecluster", "replicationgroup", "aws::elasticache"}
	case strings.Contains(s, "dynamodb"):
		return []string{"table", "aws::dynamodb"}
	case strings.Contains(s, "eks") || strings.Contains(s, "kubernetes"):
		return []string{"cluster", "nodegroup", "aws::eks"}
	case strings.Contains(s, "ecs") || strings.Contains(s, "elastic container") || strings.Contains(s, "fargate"):
		return []string{"cluster", "service", "task", "aws::ecs"}
	case strings.Contains(s, "cloudfront"):
		return []string{"distribution", "aws::cloudfront"}
	case strings.Contains(s, "sqs") || strings.Contains(s, "simple queue"):
		return []string{"queue", "aws::sqs"}
	case strings.Contains(s, "sns") || strings.Contains(s, "simple notification"):
		return []string{"topic", "aws::sns"}
	case strings.Contains(s, "opensearch") || strings.Contains(s, "elasticsearch"):
		return []string{"domain", "aws::opensearchservice", "aws::elasticsearch"}
	case strings.Contains(s, "kinesis"):
		return []string{"stream", "deliverystream", "aws::kinesis"}
	default:
		return []string{}
	}
}

func matchesService(resourceType string, hints []string) bool {
	rt := strings.ToLower(resourceType)
	for _, h := range hints {
		if strings.Contains(rt, h) {
			return true
		}
	}
	return false
}

func isCIAgent(s string) bool {
	s = strings.ToLower(s)
	for _, kw := range []string{"github-actions", "codebuild", "jenkins", "circleci", "terraform", "cdk", "deploy"} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

func dedup(ss []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
