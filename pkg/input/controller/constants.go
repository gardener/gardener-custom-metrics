// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

const (
	// ShootNamespaceLabelKey and ShootNamespaceLabelValue are used to tag each seed namespace which contains
	// a shoot
	ShootNamespaceLabelKey = "gardener.cloud/role"
	// ShootNamespaceLabelValue and ShootNamespaceLabelKey are used to tag each seed namespace which contains
	// a shoot
	ShootNamespaceLabelValue = "shoot"
)
