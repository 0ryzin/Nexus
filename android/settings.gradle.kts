// NEXUS for Android (development; not released). The app is a thin platform shell around the
// shared NEXUS core in ../app, built for arm64 as libnexus_core.so. See docs/ANDROID.md.
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "nexus-android"
include(":app")
