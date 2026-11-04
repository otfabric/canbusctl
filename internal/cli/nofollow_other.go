// SPDX-License-Identifier: MIT

//go:build !unix

package cli

// oNoFollow is not available here: CreateOutput's Lstat check alone
// refuses symbolic links.
const oNoFollow = 0
