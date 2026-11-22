plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.github.perongh.singer.internal.androidvpn"
    defaultConfig {
        consumerProguardFiles("consumer-rules.pro")
    }
    lint {
        abortOnError = true
        checkReleaseBuilds = true
        textReport = true
        textOutput = file("stdout")
        disable += setOf("GradleDependency", "OldTargetApi", "NewerVersionAvailable")
    }
}

dependencies {
    implementation("androidx.activity:activity-ktx:1.10.1")
    implementation("androidx.core:core-ktx:1.16.0")
    implementation(files("libs/libbox.aar"))
}
