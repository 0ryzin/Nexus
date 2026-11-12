import java.io.File

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// The shared NEXUS core (Go, ../../app) is built for Android arm64 with the NDK's clang as
// libnexus_core.so and packaged like any native library. The NEXUS Engine for Android
// (libnexus_engine.so) is not built yet; see docs/ANDROID.md.
val appDir = rootProject.projectDir.resolve("../app").canonicalFile
val goLibs = layout.buildDirectory.dir("nexus/jniLibs")
val nexusAssets = layout.buildDirectory.dir("nexus/assets")
val minApi = 33

fun ndkDir(): File {
    val candidates = listOfNotNull(
        System.getenv("ANDROID_NDK_HOME"), System.getenv("ANDROID_NDK_LATEST_HOME"), System.getenv("ANDROID_NDK_ROOT"),
    ).map(::File) + (System.getenv("ANDROID_HOME") ?: System.getenv("ANDROID_SDK_ROOT"))?.let { sdk ->
        File(sdk, "ndk").listFiles()?.sortedDescending() ?: emptyList()
    }.orEmpty()
    return candidates.firstOrNull { it.resolve("toolchains/llvm/prebuilt").isDirectory }
        ?: throw GradleException("The Android NDK was not found (set ANDROID_NDK_HOME)")
}

val buildNexusCore by tasks.registering(Exec::class) {
    description = "Builds the shared NEXUS core (Go) for Android arm64 as libnexus_core.so"
    val out = goLibs.get().file("arm64-v8a/libnexus_core.so").asFile
    inputs.dir(appDir).withPropertyName("nexusSource")
    outputs.file(out)
    workingDir = appDir
    doFirst {
        out.parentFile.mkdirs()
        val host = File(ndkDir(), "toolchains/llvm/prebuilt").listFiles()!!.first()
        environment("GOOS", "android")
        environment("GOARCH", "arm64")
        environment("CGO_ENABLED", "1")
        environment("CC", File(host, "bin/aarch64-linux-android$minApi-clang").absolutePath)
    }
    commandLine("go", "build", "-buildmode=c-shared", "-trimpath", "-ldflags", "-s -w", "-o", out.absolutePath, ".")
}

val nexusResources by tasks.registering(Copy::class) {
    description = "NEXUS resources and licences, extracted to the app's storage at the first start"
    from(appDir.resolve("dist-base")) {
        include("Resources/**", "Licences/**")
        exclude("**/*.psd")
    }
    into(nexusAssets.map { it.dir("nexus") })
}

android {
    namespace = "dev.nexus.android"
    compileSdk = 35
    defaultConfig {
        applicationId = "dev.nexus.android"
        minSdk = minApi
        targetSdk = 35
        versionCode = 1
        versionName = appDir.resolve("VERSION").readText().trim() + "-android-dev"
        ndk { abiFilters += "arm64-v8a" }
    }
    buildTypes {
        release {
            // no release builds are published; see docs/ANDROID.md
            isMinifyEnabled = false
        }
    }
    sourceSets["main"].jniLibs.srcDir(goLibs)
    sourceSets["main"].assets.srcDir(nexusAssets)
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    packaging { jniLibs { useLegacyPackaging = false } }
    testOptions { unitTests.isReturnDefaultValues = true }
}

tasks.named("preBuild") { dependsOn(buildNexusCore, nexusResources) }

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.webkit:webkit:1.12.1")
    testImplementation("junit:junit:4.13.2")
}
