package registry

import (
	"context"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func createInitialBucket(t testing.TB) *Bucket {
	oldEnv := os.Getenv("HCP_PACKER_BUILD_FINGERPRINT")
	os.Setenv("HCP_PACKER_BUILD_FINGERPRINT", "no-fingerprint-here")
	defer func() {
		os.Setenv("HCP_PACKER_BUILD_FINGERPRINT", oldEnv)
	}()

	t.Helper()
	bucket, err := NewBucketWithIteration(IterationOptions{})
	if err != nil {
		t.Fatalf("failed when calling NewBucketWithIteration: %s", err)
	}

	bucket.Slug = "TestBucket"
	bucket.client = &Client{
		Packer: NewMockPackerClientService(),
	}
	bucket.Iteration.RunUUID = "1234567890abcedfghijkl"

	return bucket
}

func checkError(t testing.TB, err error) {
	t.Helper()

	if err == nil {
		return
	}

	t.Errorf("received an error during testing %s", err)
}

func TestBucket_CreateInitialBuildForIteration(t *testing.T) {
	bucket := createInitialBucket(t)

	componentName := "happycloud.image"
	bucket.RegisterBuildForComponent(componentName)
	bucket.BuildLabels = map[string]string{
		"version":   "1.7.0",
		"based_off": "alpine",
	}
	err := bucket.CreateInitialBuildForIteration(context.TODO(), componentName)
	checkError(t, err)

	// Assert that a build stored on the iteration
	iBuild, ok := bucket.Iteration.builds.Load(componentName)
	if !ok {
		t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
	}

	build, ok := iBuild.(*Build)
	if !ok {
		t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
	}

	if build.ComponentType != componentName {
		t.Errorf("expected the initial build to have the defined component type")
	}

	if diff := cmp.Diff(build.Labels, bucket.BuildLabels); diff != "" {
		t.Errorf("expected the initial build to have the defined build labels %v", diff)
	}
}

func TestBucket_UpdateLabelsForBuild(t *testing.T) {
	tc := []struct {
		desc              string
		buildName         string
		bucketBuildLabels map[string]string
		buildLabels       map[string]string
		labelsCount       int
		noDiffExpected    bool
	}{
		{
			desc:           "no bucket or build specific labels",
			buildName:      "happcloud.image",
			noDiffExpected: true,
		},
		{
			desc:      "bucket build labels",
			buildName: "happcloud.image",
			bucketBuildLabels: map[string]string{
				"version":   "1.7.0",
				"based_off": "alpine",
			},
			labelsCount:    2,
			noDiffExpected: true,
		},
		{
			desc:      "bucket build labels and build specific label",
			buildName: "happcloud.image",
			bucketBuildLabels: map[string]string{
				"version":   "1.7.0",
				"based_off": "alpine",
			},
			buildLabels: map[string]string{
				"source_image": "another-happycloud-image",
			},
			labelsCount:    3,
			noDiffExpected: false,
		},
		{
			desc:      "build specific label",
			buildName: "happcloud.image",
			buildLabels: map[string]string{
				"source_image": "another-happycloud-image",
			},
			labelsCount:    1,
			noDiffExpected: false,
		},
	}

	for _, tt := range tc {
		tt := tt
		t.Run(tt.desc, func(t *testing.T) {
			bucket := createInitialBucket(t)

			componentName := tt.buildName
			bucket.RegisterBuildForComponent(componentName)

			for k, v := range tt.bucketBuildLabels {
				bucket.BuildLabels[k] = v
			}

			err := bucket.CreateInitialBuildForIteration(context.TODO(), componentName)
			checkError(t, err)

			err = bucket.UpdateLabelsForBuild(componentName, tt.buildLabels)
			checkError(t, err)

			// Assert that the build is stored on the iteration
			iBuild, ok := bucket.Iteration.builds.Load(componentName)
			if !ok {
				t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
			}

			build, ok := iBuild.(*Build)
			if !ok {
				t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
			}

			if build.ComponentType != componentName {
				t.Errorf("expected the build to have the defined component type")
			}

			if len(build.Labels) != tt.labelsCount {
				t.Errorf("expected the build to have %d build labels but there is only: %d", tt.labelsCount, len(build.Labels))
			}

			diff := cmp.Diff(build.Labels, bucket.BuildLabels)
			if (diff == "") != tt.noDiffExpected {
				t.Errorf("expected the build to have an additional build label but there is no diff: %q", diff)
			}

		})
	}
}

func TestBucket_UpdateLabelsForBuild_withMultipleBuilds(t *testing.T) {
	bucket := createInitialBucket(t)

	firstComponent := "happycloud.image"
	bucket.RegisterBuildForComponent(firstComponent)
	err := bucket.CreateInitialBuildForIteration(context.TODO(), firstComponent)
	checkError(t, err)

	secondComponent := "happycloud.image2"
	bucket.RegisterBuildForComponent(secondComponent)
	err = bucket.CreateInitialBuildForIteration(context.TODO(), secondComponent)
	checkError(t, err)

	err = bucket.UpdateLabelsForBuild(firstComponent, map[string]string{
		"source_image": "another-happycloud-image",
	})
	checkError(t, err)

	err = bucket.UpdateLabelsForBuild(secondComponent, map[string]string{
		"source_image": "the-original-happycloud-image",
		"role_name":    "no-role-is-a-good-role",
	})
	checkError(t, err)

	var registeredBuilds []*Build
	expectedComponents := []string{firstComponent, secondComponent}
	for _, componentName := range expectedComponents {
		// Assert that a build stored on the iteration
		iBuild, ok := bucket.Iteration.builds.Load(componentName)
		if !ok {
			t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
		}

		build, ok := iBuild.(*Build)
		if !ok {
			t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
		}
		registeredBuilds = append(registeredBuilds, build)

		if build.ComponentType != componentName {
			t.Errorf("expected the initial build to have the defined component type")
		}

		if ok := cmp.Equal(build.Labels, bucket.BuildLabels); ok {
			t.Errorf("expected the build to have an additional build label but they are equal")
		}
	}

	if len(registeredBuilds) != 2 {
		t.Errorf("expected the bucket to have 2 registered builds but got %d", len(registeredBuilds))
	}

	if ok := cmp.Equal(registeredBuilds[0].Labels, registeredBuilds[1].Labels); ok {
		t.Errorf("expected registered builds to have different labels but they are equal")
	}
}

func TestBucket_PopulateIteration(t *testing.T) {
	tc := []struct {
		desc              string
		buildName         string
		bucketBuildLabels map[string]string
		buildLabels       map[string]string
		labelsCount       int
		buildCompleted    bool
		noDiffExpected    bool
	}{
		{
			desc:      "existing incomplete build",
			buildName: "happcloud.image",
			bucketBuildLabels: map[string]string{
				"version":   "1.7.0",
				"based_off": "alpine",
			},
			labelsCount:    2,
			buildCompleted: false,
			noDiffExpected: true,
		},
		{
			desc:      "existing incomplete build with improperly set build labels",
			buildName: "happcloud.image",
			bucketBuildLabels: map[string]string{
				"version":   "1.7.3",
				"based_off": "alpine-3.14",
			},
			buildLabels: map[string]string{
				"version":   "packer.version",
				"based_off": "var.distro",
			},
			labelsCount:    2,
			buildCompleted: false,
			noDiffExpected: true,
		},
		{
			desc:      "completed build with no labels",
			buildName: "happcloud.image",
			bucketBuildLabels: map[string]string{
				"version":   "1.7.0",
				"based_off": "alpine",
			},
			labelsCount:    0,
			buildCompleted: true,
			noDiffExpected: false,
		},
		{
			desc:      "existing incomplete build with extra previously set build label",
			buildName: "happcloud.image",
			bucketBuildLabels: map[string]string{
				"version":   "1.7.3",
				"based_off": "alpine-3.14",
			},
			buildLabels: map[string]string{
				"arch": "linux/386",
			},
			labelsCount:    3,
			buildCompleted: false,
			noDiffExpected: false,
		},
	}

	for _, tt := range tc {
		tt := tt
		t.Run(tt.desc, func(t *testing.T) {

			mockService := NewMockPackerClientService()
			mockService.BucketAlreadyExist = true
			mockService.IterationAlreadyExist = true
			mockService.BuildAlreadyDone = tt.buildCompleted

			bucket, err := NewBucketWithIteration(IterationOptions{})
			if err != nil {
				t.Fatalf("failed when calling NewBucketWithIteration: %s", err)
			}

			bucket.Slug = "TestBucket"
			bucket.client = &Client{
				Packer: mockService,
			}
			for k, v := range tt.bucketBuildLabels {
				bucket.BuildLabels[k] = v
			}

			componentName := "happycloud.image"
			bucket.RegisterBuildForComponent(componentName)

			mockService.ExistingBuilds = append(mockService.ExistingBuilds, componentName)
			mockService.ExistingBuildLabels = tt.buildLabels

			err = bucket.PopulateIteration(context.TODO())
			checkError(t, err)

			// Assert that a build stored on the iteration
			iBuild, ok := bucket.Iteration.builds.Load(componentName)
			if !ok {
				t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
			}

			build, ok := iBuild.(*Build)
			if !ok {
				t.Errorf("expected an initial build for %s to be created, but it failed", componentName)
			}

			if build.ComponentType != componentName {
				t.Errorf("expected the initial build to have the defined component type")
			}

			if len(build.Labels) != tt.labelsCount {
				t.Errorf("expected the build to have %d build labels but there is only: %d", tt.labelsCount, len(build.Labels))
			}

			diff := cmp.Diff(build.Labels, bucket.BuildLabels)
			if (diff == "") != tt.noDiffExpected {
				t.Errorf("expected the build to have bucket build labels but there is no diff: %q", diff)
			}
		})
	}
}
