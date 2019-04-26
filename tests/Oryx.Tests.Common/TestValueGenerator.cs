﻿// --------------------------------------------------------------------------------------------
// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.
// --------------------------------------------------------------------------------------------

using System;
using System.Collections.Generic;
using System.Text;
using System.Linq;

namespace Microsoft.Oryx.Tests.Common
{
    public static class TestValueGenerator
    {
        private readonly static List<string> NodeVersions = new List<string>
        {
            "4.4", "4.5", "4.8",
            "6.2", "6.6", "6.9", "6.10", "6.11",
            "8.0", "8.1", "8.2", "8.8", "8.9", "8.11", "8.12",
            "9.4",
            "10.1", "10.10", "10.14"
        };

        private readonly static List<string> ZipOptions = new List<string>
        {
            "tar-gz", "zip"
        };

        public static IEnumerable<object[]> GetZipOptions_NodeVersions()
        {
            foreach (var version in NodeVersions)
            {
                foreach (var zipOption in ZipOptions)
                {
                    yield return new object[] { zipOption, version };
                }
            }
        }

        public static IEnumerable<object[]> GetNodeVersions_SupportDebugging()
        {
            var versions = new List<string>
            {
                "8.0", "8.1", "8.2", "8.8", "8.9", "8.11", "8.12",
                "9.4",
                "10.1", "10.10", "10.14"
            };

            return versions.Select(v => new object[] { v });
        }

        public static IEnumerable<object[]> GetNodeVersions_DoesNotSupportDebugging()
        {
            var versions = new List<string>
            {
                "4.4.7", "4.5.0", "6.2.2", "6.9.3", "6.10.3", "6.11.0"
            };

            return versions.Select(v => new object[] { v });
        }

        public static IEnumerable<object[]> GetNodeVersions()
        {
            foreach (var version in NodeVersions)
            {
                yield return new object[] { version };
            }
        }

        public static IEnumerable<object[]> GetNodeVersions_SupportPm2()
        {
            return NodeVersions
                .Where(v => !v.StartsWith("4."))
                .Select(v => new object[] { v });
        }
    }
}
