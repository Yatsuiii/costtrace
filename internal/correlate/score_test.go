package correlate

import (
	"reflect"
	"sort"
	"testing"
)

func TestServiceToResourceHints(t *testing.T) {
	cases := []struct {
		service   string
		wantAny   []string // at least one of these substrings must appear in any returned hint
		wantEmpty bool
	}{
		{service: "Amazon Elastic Compute Cloud - Compute", wantAny: []string{"instance", "aws::ec2"}},
		{service: "EC2 - Other", wantAny: []string{"instance"}},
		{service: "Amazon Simple Storage Service", wantAny: []string{"bucket"}},
		{service: "Amazon Relational Database Service", wantAny: []string{"dbinstance", "dbcluster"}},
		{service: "AWS Lambda", wantAny: []string{"function"}},
		{service: "Elastic Load Balancing", wantAny: []string{"loadbalancer"}},
		{service: "Amazon ElastiCache", wantAny: []string{"cachecluster"}},
		{service: "Amazon DynamoDB", wantAny: []string{"table"}},
		{service: "Amazon EKS", wantAny: []string{"cluster", "nodegroup"}},
		{service: "Amazon Elastic Container Service", wantAny: []string{"cluster", "service"}},
		{service: "Amazon CloudFront", wantAny: []string{"distribution"}},
		{service: "Amazon Simple Queue Service", wantAny: []string{"queue"}},
		{service: "Amazon Simple Notification Service", wantAny: []string{"topic"}},
		{service: "Amazon OpenSearch Service", wantAny: []string{"domain"}},
		{service: "Amazon Kinesis", wantAny: []string{"stream"}},
		{service: "Some Brand New Service", wantEmpty: true},
	}
	for _, c := range cases {
		got := serviceToResourceHints(c.service)
		if c.wantEmpty {
			if len(got) != 0 {
				t.Errorf("%q: expected empty hints, got %v", c.service, got)
			}
			continue
		}
		var found bool
		for _, want := range c.wantAny {
			for _, g := range got {
				if g == want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%q: hints %v missing all of expected %v", c.service, got, c.wantAny)
		}
	}
}

func TestMatchesService(t *testing.T) {
	cases := []struct {
		resourceType string
		hints        []string
		want         bool
	}{
		{"AWS::EC2::Instance", []string{"instance", "aws::ec2"}, true},
		{"AWS::EC2::Volume", []string{"instance", "volume"}, true},
		{"AWS::S3::Bucket", []string{"instance"}, false},
		{"", []string{"instance"}, false},
		{"AWS::EC2::Instance", []string{}, false},
	}
	for _, c := range cases {
		got := matchesService(c.resourceType, c.hints)
		if got != c.want {
			t.Errorf("matchesService(%q, %v) = %v, want %v", c.resourceType, c.hints, got, c.want)
		}
	}
}

func TestIsCIAgent(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"github-actions-runner", true},
		{"GitHub-Actions-Bot", true},
		{"AWSCodeBuild-deploy", true},
		{"jenkins-master", true},
		{"circleci-agent", true},
		{"terraform/1.5.0", true},
		{"aws-cdk", true},
		{"my-deploy-role", true},
		{"john.doe@example.com", false},
		{"AWSReservedSSO_AdministratorAccess", false},
		{"", false},
		{"random-application", false},
	}
	for _, c := range cases {
		if got := isCIAgent(c.s); got != c.want {
			t.Errorf("isCIAgent(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestDedup(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{[]string{"a", "a", "b", "a"}, []string{"a", "b"}},
		{[]string{}, nil},
		{[]string{"x", "x", "x"}, []string{"x"}},
	}
	for _, c := range cases {
		got := dedup(c.in)
		// dedup preserves first-seen order; sort both for stable comparison
		sort.Strings(got)
		sort.Strings(c.want)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("dedup(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
