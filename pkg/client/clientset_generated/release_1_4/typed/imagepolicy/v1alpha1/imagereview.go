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

package v1alpha1

// ImageReviewsGetter has a method to return a ImageReviewInterface.
// A group's client should implement this interface.
type ImageReviewsGetter interface {
	ImageReviews() ImageReviewInterface
}

// ImageReviewInterface has methods to work with ImageReview resources.
type ImageReviewInterface interface {
	ImageReviewExpansion
}

// imageReviews implements ImageReviewInterface
type imageReviews struct {
	client *ImagepolicyClient
}

// newImageReviews returns a ImageReviews
func newImageReviews(c *ImagepolicyClient) *imageReviews {
	return &imageReviews{
		client: c,
	}
}
