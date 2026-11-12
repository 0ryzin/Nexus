package dev.nexus.android

import android.content.Context
import java.io.File

/**
 * NEXUS's resources (sounds, icons, themes, profile icons) and licence texts ship in the APK's
 * assets and are copied to <filesDir>/nexus once per app version, where the core reads them
 * exactly as it reads the Windows program folder. User data is in <filesDir>/nexus-data and is
 * never touched here.
 */
object BundledFiles {
    fun install(context: Context): File {
        val root = File(context.filesDir, "nexus")
        val stamp = File(root, ".installed-version")
        val version = context.packageManager.getPackageInfo(context.packageName, 0).longVersionCode.toString() +
            ":" + (context.packageManager.getPackageInfo(context.packageName, 0).lastUpdateTime)
        if (stamp.isFile && stamp.readText() == version) return root
        for (top in listOf("Resources", "Licences")) {
            File(root, top).deleteRecursively()
            copy(context, "nexus/$top", File(root, top))
        }
        root.mkdirs()
        stamp.writeText(version)
        return root
    }

    private fun copy(context: Context, asset: String, dst: File) {
        val children = context.assets.list(asset) ?: emptyArray()
        if (children.isEmpty()) { // a file
            dst.parentFile?.mkdirs()
            context.assets.open(asset).use { input -> dst.outputStream().use { input.copyTo(it) } }
            return
        }
        dst.mkdirs()
        for (c in children) copy(context, "$asset/$c", File(dst, c))
    }
}
