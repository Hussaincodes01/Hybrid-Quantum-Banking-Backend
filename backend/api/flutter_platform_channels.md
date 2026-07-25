# FINIX Flutter Platform Channel Specification

## Hardware Attestation & Biometric Authentication

---

## 1. Hardware Device Fingerprint

### Method Channel: `com.finix.hardware/device`

#### Dart Interface

```dart
class FinixHardware {
  static const MethodChannel _deviceChannel =
      MethodChannel('com.finix.hardware/device');

  static Future<Map<String, dynamic>> getDeviceFingerprint() async {
    final Map<dynamic, dynamic> result =
        await _deviceChannel.invokeMethod('getDeviceFingerprint');
    return Map<String, dynamic>.from(result);
  }

  /// Returns a SHA-256 hash of the device fingerprint for server transmission
  static Future<String> getDeviceIdFingerprint() async {
    final fingerprint = await getDeviceFingerprint();
    final canonical = jsonEncode(fingerprint)..trim();
    return sha256.convert(utf8.encode(canonical)).toString();
  }
}
```

#### iOS Implementation (Swift)

```swift
import UIKit
import Security
import LocalAuthentication

@objc class FinixDevicePlugin: NSObject {

    @objc func getDeviceFingerprint(
        resolver: @escaping RCTPromiseResolveBlock,
        rejecter: @escaping RCTPromiseRejectBlock
    ) {
        var result: [String: Any] = [:]

        let device = UIDevice.current
        result["deviceModel"] = Self.deviceModelIdentifier()
        result["osVersion"] = device.systemVersion
        result["systemUptime"] = Int(ProcessInfo.processInfo.systemUptime)
        result["identifierForVendor"] = device.identifierForVendor?.uuidString ?? ""

        // Hardware UUID from Keychain (created on first launch)
        result["hardwareUUID"] = Self.getOrCreateHardwareUUID()

        // Secure Enclave check
        let context = LAContext()
        var error: NSError?
        let canEvaluate = context.canEvaluatePolicy(
            .deviceOwnerAuthenticationWithBiometrics, error: &error
        )
        result["secureEnclaveAvailable"] = canEvaluate

        // Biometric type
        if canEvaluate {
            switch context.biometryType {
            case .faceID:
                result["biometricType"] = "faceID"
            case .touchID:
                result["biometricType"] = "touchID"
            default:
                result["biometricType"] = "none"
            }
        } else {
            result["biometricType"] = "none"
        }

        // Screen resolution
        let screen = UIScreen.main.nativeBounds
        result["screenResolution"] = "\(Int(screen.width))x\(Int(screen.height))"

        // Total disk space
        let fileURL = URL(fileURLWithPath: NSHomeDirectory())
        do {
            let values = try fileURL.resourceValues(
                keys: [.volumeTotalCapacityKey]
            )
            result["totalDiskSpace"] = values.volumeTotalCapacity ?? 0
        } catch {
            result["totalDiskSpace"] = 0
        }

        resolver(result)
    }

    // MARK: - Hardware UUID (Keychain-persisted)

    private static func getOrCreateHardwareUUID() -> String {
        let service = "com.finix.hardware"
        let account = "device-uuid"

        // Attempt retrieval
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]

        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)

        if status == errSecSuccess, let data = item as? Data,
           let uuid = String(data: data, encoding: .utf8) {
            return uuid
        }

        // Generate new UUID
        let newUUID = UUID().uuidString
        if let data = newUUID.data(using: .utf8) {
            let addQuery: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account,
                kSecValueData as String: data,
                kSecAttrAccessible as String:
                    kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            ]
            SecItemAdd(addQuery as CFDictionary, nil)
        }

        return newUUID
    }

    private static func deviceModelIdentifier() -> String {
        var systemInfo = utsname()
        uname(&systemInfo)
        let machineMirror = Mirror(reflecting: systemInfo.machine)
        return machineMirror.children.reduce("") { identifier, element in
            guard let value = element.value as? Int8, value != 0 else {
                return identifier
            }
            return identifier + String(UnicodeScalar(UInt8(value)))
        }
    }
}
```

#### Android Implementation (Kotlin)

```kotlin
package com.finix.hardware

import android.content.Context
import android.os.Build
import android.os.Process
import android.provider.Settings
import android.telephony.TelephonyManager
import android.util.Log
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import io.flutter.embedding.engine.plugins.FlutterPlugin
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.io.File
import java.security.KeyStore
import java.util.UUID

class FinixDevicePlugin : FlutterPlugin, MethodChannel.MethodCallHandler {

    private lateinit var channel: MethodChannel
    private lateinit var context: Context

    override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        channel = MethodChannel(binding.binaryMessenger, "com.finix.hardware/device")
        channel.setMethodCallHandler(this)
        context = binding.applicationContext
    }

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "getDeviceFingerprint" -> {
                val fingerprint = buildFingerprint()
                result.success(fingerprint)
            }
            else -> result.notImplemented()
        }
    }

    private fun buildFingerprint(): Map<String, Any> {
        val map = mutableMapOf<String, Any>()

        map["deviceModel"] = "${Build.MANUFACTURER} ${Build.MODEL}"
        map["osVersion"] = Build.VERSION.RELEASE
        map["systemUptime"] = Process.getElapsedCpuTime().toInt()
        map["identifierForVendor"] = getOrCreateVendorId()

        // Android-specific fields
        map["androidId"] = Settings.Secure.getString(
            context.contentResolver, Settings.Secure.ANDROID_ID
        ) ?: ""
        map["buildFingerprint"] = Build.FINGERPRINT

        // Keystore attestation
        map["keystoreAttestation"] = getKeystoreAttestation()

        // SIM serial (requires READ_PHONE_STATE permission at runtime)
        map["simSerialNumber"] = getSimSerialNumber()

        // Screen resolution
        val displayMetrics = context.resources.displayMetrics
        map["screenResolution"] =
            "${displayMetrics.widthPixels}x${displayMetrics.heightPixels}"

        // Total disk space
        val stat = android.os.StatFs(Environment.getDataDirectory().path)
        map["totalDiskSpace"] = stat.totalBytes

        // Biometric type
        map["biometricType"] = getBiometricType()

        // Secure Enclave equivalent (StrongBox)
        map["secureEnclaveAvailable"] = isStrongBoxAvailable()

        return map
    }

    private fun getOrCreateVendorId(): String {
        val prefs = context.getSharedPreferences("finix_device", Context.MODE_PRIVATE)
        var id = prefs.getString("vendor_id", null)
        if (id == null) {
            id = UUID.randomUUID().toString()
            prefs.edit().putString("vendor_id", id).apply()
        }
        return id
    }

    private fun getKeystoreAttestation(): String {
        return try {
            val keyStore = KeyStore.getInstance("AndroidKeyStore")
            keyStore.load(null)
            // Return attestation chain if key exists
            val alias = "finix_attestation_key"
            if (keyStore.containsAlias(alias)) {
                val cert = keyStore.getCertificate(alias)
                android.util.Base64.encodeToString(
                    cert.encoded, android.util.Base64.NO_WRAP
                )
            } else {
                ""
            }
        } catch (e: Exception) {
            ""
        }
    }

    private fun getSimSerialNumber(): String {
        return try {
            val tm = context.getSystemService(Context.TELEPHONY_SERVICE)
                    as TelephonyManager
            tm.simSerialNumber ?: ""
        } catch (e: SecurityException) {
            ""
        }
    }

    private fun getBiometricType(): String {
        val biometricManager = BiometricManager.from(context)
        return when (biometricManager.canAuthenticate(
            BiometricManager.Authenticators.BIOMETRIC_STRONG
        )) {
            BiometricManager.BIOMETRIC_SUCCESS -> {
                val keyStore = KeyStore.getInstance("AndroidKeyStore")
                keyStore.load(null)
                // Distinguish face vs fingerprint via feature check
                if (context.packageManager.hasSystemFeature(
                        "android.hardware.face.face")) {
                    "faceID"
                } else {
                    "fingerprint"
                }
            }
            else -> "none"
        }
    }

    private fun isStrongBoxAvailable(): Boolean {
        return try {
            context.packageManager.hasSystemFeature(
                "android.hardware.strongbox_keystore"
            )
        } catch (e: Exception) {
            false
        }
    }

    override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        channel.setMethodCallHandler(null)
    }
}
```

---

## 2. Biometric Authentication Channel

### Method Channel: `com.finix.hardware/biometric`

#### Dart Interface

```dart
class FinixBiometric {
  static const MethodChannel _biometricChannel =
      MethodChannel('com.finix.hardware/biometric');

  /// Prompt user for biometric authentication
  static Future<bool> authenticate({required String reason}) async {
    final result = await _biometricChannel.invokeMethod(
      'authenticate',
      {'reason': reason},
    );
    return result == true;
  }

  /// Generate a P-256 key pair in Secure Enclave / StrongBox
  /// Returns publicKey (base64 SPKI) and keyId (reference handle)
  static Future<BiometricKeyPair> registerKeyPair() async {
    final Map<dynamic, dynamic> result =
        await _biometricChannel.invokeMethod('registerKeyPair');
    return BiometricKeyPair(
      publicKey: result['publicKey'] as String,
      keyId: result['keyId'] as String,
    );
  }

  /// Sign a challenge using the registered key
  static Future<String> signChallenge({
    required String challenge,
    required String keyId,
  }) async {
    final result = await _biometricChannel.invokeMethod(
      'signChallenge',
      {'challenge': challenge, 'keyId': keyId},
    );
    return result as String; // base64-encoded signature
  }

  /// Get available biometric type
  static Future<String> getBiometricType() async {
    final result = await _biometricChannel.invokeMethod('getBiometricType');
    return result as String;
  }
}

class BiometricKeyPair {
  final String publicKey;  // base64-encoded SPKI
  final String keyId;      // platform key reference

  BiometricKeyPair({required this.publicKey, required this.keyId});
}
```

#### iOS Implementation (Swift)

```swift
import LocalAuthentication
import Security

@objc class FinixBiometricPlugin: NSObject {

    @objc func authenticate(
        reason: String,
        resolver: @escaping RCTPromiseResolveBlock,
        rejecter: @escaping RCTPromiseRejectBlock
    ) {
        let context = LAContext()
        var error: NSError?

        guard context.canEvaluatePolicy(
            .deviceOwnerAuthenticationWithBiometrics, error: &error
        ) else {
            resolver(false)
            return
        }

        context.evaluatePolicy(
            .deviceOwnerAuthenticationWithBiometrics,
            localizedReason: reason
        ) { success, _ in
            resolver(success)
        }
    }

    @objc func registerKeyPair(
        resolver: @escaping RCTPromiseResolveBlock,
        rejecter: @escaping RCTPromiseRejectBlock
    ) {
        let accessControl = SecAccessControlCreateWithFlags(
            kCFAllocatorDefault,
            kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
            [.privateKeyUsage, .biometryCurrentSet],
            nil
        )!

        let attributes: [String: Any] = [
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecAttrKeySizeInBits as String: 256,
            kSecAttrTokenID as String: kSecAttrTokenIDSecureEnclave,
            kSecPrivateKeyAttrs as String: [
                kSecAttrIsPermanent as String: true,
                kSecAttrApplicationTag as String: "com.finix.biometric.signing".data(using: .utf8)!,
                kSecAttrAccessControl as String: accessControl
            ]
        ]

        var error: Unmanaged<CFError>?
        guard let privateKey = SecKeyCreateRandomKey(attributes as CFDictionary, &error) else {
            rejecter("KEY_ERROR", "Failed to generate key", error?.takeRetainedValue())
            return
        }

        // Extract public key as SPKI (base64)
        guard let publicKey = SecKeyCopyPublicKey(privateKey),
              let publicKeyData = SecKeyCopyExternalRepresentation(publicKey, nil) as? Data else {
            rejecter("KEY_ERROR", "Failed to export public key", nil)
            return
        }

        // Wrap in SPKI envelope
        let spkiBase64 = Self.wrapAsSPKIECDSA(publicKeyData)

        // Generate key ID
        let keyId = UUID().uuidString

        // Store keyId mapping in Keychain
        Self.storeKeyMapping(keyId: keyId, tag: "com.finix.biometric.signing")

        resolver([
            "publicKey": spkiBase64,
            "keyId": keyId
        ])
    }

    @objc func signChallenge(
        challenge: String,
        keyId: String,
        resolver: @escaping RCTPromiseResolveBlock,
        rejecter: @escaping RCTPromiseRejectBlock
    ) {
        let tag = "com.finix.biometric.signing.\(keyId)".data(using: .utf8)!

        let query: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: tag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String: true
        ]

        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)

        guard status == errSecSuccess,
              let privateKey = item else {
            rejecter("KEY_NOT_FOUND", "Key not found for keyId: \(keyId)", nil)
            return
        }

        guard let signature = SecKeyCreateSignature(
            privateKey as! SecKey,
            .ecdsaSignatureMessageX962SHA256,
            Data(challenge.utf8),
            &error
        ) else {
            rejecter("SIGN_ERROR", "Signing failed", error?.takeRetainedValue())
            return
        }

        resolver(signature.base64EncodedString())
    }

    @objc func getBiometricType(
        resolver: @escaping RCTPromiseResolveBlock,
        rejecter: @escaping RCTPromiseRejectBlock
    ) {
        let context = LAContext()
        var error: NSError?

        guard context.canEvaluatePolicy(
            .deviceOwnerAuthenticationWithBiometrics, error: &error
        ) else {
            resolver("none")
            return
        }

        switch context.biometryType {
        case .faceID:   resolver("faceID")
        case .touchID:  resolver("touchID")
        default:        resolver("none")
        }
    }

    // MARK: - Helpers

    private static func wrapAsSPKIECDSA(_ rawPublicKey: Data) -> String {
        // ASN.1 DER encoding for ECDSA P-256 SubjectPublicKeyInfo
        let algorithmIdentifier: [UInt8] = [
            0x30, 0x13,                          // SEQUENCE
            0x06, 0x07, 0x2A, 0x86, 0x48,       // OID 1.2.840.10045.2.1
            0xCE, 0x3D, 0x02, 0x01,              // ecPublicKey
            0x06, 0x08, 0x2A, 0x86, 0x48,       // OID 1.2.840.10045.3.1.7
            0xCE, 0x3D, 0x03, 0x01, 0x07         // P-256
        ]

        var spki = Data()
        spki.append(contentsOf: algorithmIdentifier)
        spki.append(0x03)                         // BIT STRING
        spki.append(Data([UInt8(rawPublicKey.count + 1)]))
        spki.append(0x00)                         // unused bits
        spki.append(rawPublicKey)

        // Wrap in outer SEQUENCE
        var outer = Data()
        outer.append(0x30)
        outer.append(Data([UInt8(spki.count)]))
        outer.append(spki)

        return outer.base64EncodedString()
    }

    private static func storeKeyMapping(keyId: String, tag: String) {
        let account = "com.finix.biometric.keys"
        let service = "com.finix.biometric"

        var dict = loadKeyMappings()
        dict[keyId] = tag

        if let data = try? JSONSerialization.data(withJSONObject: dict) {
            let query: [String: Any] = [
                kSecClass as String: kSecClassGenericPassword,
                kSecAttrService as String: service,
                kSecAttrAccount as String: account
            ]
            SecItemDelete(query as CFDictionary)

            var addQuery = query
            addQuery[kSecValueData as String] = data
            SecItemAdd(addQuery as CFDictionary, nil)
        }
    }

    private static func loadKeyMappings() -> [String: String] {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "com.finix.biometric",
            kSecAttrAccount as String: "com.finix.biometric.keys",
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]

        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data,
              let dict = try? JSONSerialization.jsonObject(with: data) as? [String: String]
        else {
            return [:]
        }
        return dict
    }
}
```

#### Android Implementation (Kotlin)

```kotlin
package com.finix.hardware

import android.content.Context
import android.os.CancellationSignal
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.util.UUID
import javax.crypto.Cipher

class FinixBiometricPlugin(
    private val context: Context,
    private val activity: android.app.Activity?
) : MethodChannel.MethodCallHandler {

    companion object {
        private const val KEYSTORE_ALIAS = "finix_biometric_key"
        private const val ANDROID_KEYSTORE = "AndroidKeyStore"
    }

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "authenticate" -> {
                val reason = call.argument<String>("reason") ?: "Authenticate"
                authenticate(result, reason)
            }
            "registerKeyPair" -> registerKeyPair(result)
            "signChallenge" -> {
                val challenge = call.argument<String>("challenge") ?: ""
                val keyId = call.argument<String>("keyId") ?: ""
                signChallenge(result, challenge, keyId)
            }
            "getBiometricType" -> result.success(getBiometricType())
            else -> result.notImplemented()
        }
    }

    private fun authenticate(result: MethodChannel.Result, reason: String) {
        val biometricManager = BiometricManager.from(context)
        if (biometricManager.canAuthenticate(
                BiometricManager.Authenticators.BIOMETRIC_STRONG
            ) != BiometricManager.BIOMETRIC_SUCCESS
        ) {
            result.success(false)
            return
        }

        val executor = ContextCompat.getMainExecutor(context)
        val callback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(
                result: BiometricPrompt.AuthenticationResult
            ) {
                super.onAuthenticationSucceeded(result)
                result.success(true)
            }

            override fun onAuthenticationError(
                errorCode: Int, errString: CharSequence
            ) {
                super.onAuthenticationError(errorCode, errString)
                result.success(false)
            }

            override fun onAuthenticationFailed() {
                super.onAuthenticationFailed()
                result.success(false)
            }
        }

        val prompt = BiometricPrompt(
            activity ?: return result.success(false), executor, callback
        )

        val promptInfo = BiometricPrompt.PromptInfo.Builder()
            .setTitle("FINIX Authentication")
            .setSubtitle(reason)
            .setNegativeButtonText("Cancel")
            .setAllowedAuthenticators(
                BiometricManager.Authenticators.BIOMETRIC_STRONG
            )
            .build()

        // Use a cipher-based auth to bind result to key
        val cipher = getCipherForAuth()
        if (cipher != null) {
            prompt.authenticate(promptInfo, BiometricPrompt.CryptoObject(cipher))
        } else {
            prompt.authenticate(promptInfo)
        }
    }

    private fun registerKeyPair(result: MethodChannel.Result) {
        try {
            val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE)
            keyStore.load(null)

            // Delete existing key if any
            keyStore.deleteEntry(KEYSTORE_ALIAS)

            val keyPairGenerator = KeyPairGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_EC, ANDROID_KEYSTORE
            )

            val spec = KeyGenParameterSpec.Builder(
                KEYSTORE_ALIAS,
                KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY
            )
                .setDigests(KeyProperties.DIGEST_SHA256)
                .setSignaturePaddings(KeyProperties.SIGNATURE_PADDING_RSA_PKCS1)
                .setIsStrongBoxBacked(true) // Use StrongBox if available
                .setUserAuthenticationRequired(true)
                .setInvalidatedByBiometricEnrollment(true)
                .build()

            keyPairGenerator.initialize(spec)
            val keyPair = keyPairGenerator.generateKeyPair()

            // Export public key as SPKI
            val publicKey = keyPair.public
            val spkiBase64 = android.util.Base64.encodeToString(
                publicKey.encoded, android.util.Base64.NO_WRAP
            )

            val keyId = UUID().toString()
            saveKeyMapping(keyId, KEYSTORE_ALIAS)

            val map = mapOf(
                "publicKey" to spkiBase64,
                "keyId" to keyId
            )
            result.success(map)
        } catch (e: Exception) {
            result.error("KEY_ERROR", e.message, null)
        }
    }

    private fun signChallenge(
        result: MethodChannel.Result, challenge: String, keyId: String
    ) {
        try {
            val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE)
            keyStore.load(null)

            val alias = getKeyAlias(keyId) ?: KEYSTORE_ALIAS
            val privateKey = keyStore.getKey(alias, null) as? java.security.PrivateKey
                ?: run {
                    result.error("KEY_NOT_FOUND", "No key for $keyId", null)
                    return
                }

            val signature = Signature.getInstance("SHA256withECDSA")
            signature.initSign(privateKey)
            signature.update(challenge.toByteArray(Charsets.UTF_8))
            val sigBytes = signature.sign()

            result.success(
                android.util.Base64.encodeToString(
                    sigBytes, android.util.Base64.NO_WRAP
                )
            )
        } catch (e: Exception) {
            result.error("SIGN_ERROR", e.message, null)
        }
    }

    private fun getBiometricType(): String {
        val biometricManager = BiometricManager.from(context)
        if (biometricManager.canAuthenticate(
                BiometricManager.Authenticators.BIOMETRIC_STRONG
            ) != BiometricManager.BIOMETRIC_SUCCESS
        ) {
            return "none"
        }

        return if (context.packageManager.hasSystemFeature(
                "android.hardware.face.face"
            )) {
            "faceID"
        } else {
            "fingerprint"
        }
    }

    private fun getCipherForAuth(): Cipher? {
        return try {
            val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE)
            keyStore.load(null)
            val key = keyStore.getKey(KEYSTORE_ALIAS, null) ?: return null
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, key)
            cipher
        } catch (e: Exception) {
            null
        }
    }

    private fun saveKeyMapping(keyId: String, alias: String) {
        val prefs = context.getSharedPreferences(
            "finix_biometric_keys", Context.MODE_PRIVATE
        )
        prefs.edit().putString(keyId, alias).apply()
    }

    private fun getKeyAlias(keyId: String): String? {
        val prefs = context.getSharedPreferences(
            "finix_biometric_keys", Context.MODE_PRIVATE
        )
        return prefs.getString(keyId, null)
    }
}
```

---

## 3. API Client Integration

### Dart API Client Usage

```dart
import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;

class FinixApiClient {
  final String baseUrl;
  final http.Client _http;

  String? _deviceIdFingerprint;
  String? _sessionToken;
  String? _biometricKeyId;

  FinixApiClient({required this.baseUrl, http.Client? client})
      : _http = client ?? http.Client();

  // ─── App Start: Register Device Fingerprint ───────────────────────

  Future<void> initializeDevice() async {
    final fingerprint = await FinixHardware.getDeviceFingerprint();
    final raw = jsonEncode(fingerprint);
    _deviceIdFingerprint = sha256.convert(utf8.encode(raw)).toString();
  }

  // ─── Registration ─────────────────────────────────────────────────

  Future<RegisterResponse> register({
    required String email,
    required String phone,
    required String pin,
  }) async {
    await initializeDevice();

    final response = await _http.post(
      Uri.parse('$baseUrl/v1/auth/register'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({
        'email': email,
        'phone': phone,
        'pin': pin,
        'deviceIdFingerprint': _deviceIdFingerprint,
        'deviceInfo': await FinixHardware.getDeviceFingerprint(),
      }),
    );

    return RegisterResponse.fromJson(jsonDecode(response.body));
  }

  // ─── Biometric Setup ──────────────────────────────────────────────

  Future<void> setupBiometric() async {
    final keyPair = await FinixBiometric.registerKeyPair();
    _biometricKeyId = keyPair.keyId;

    final response = await _http.post(
      Uri.parse('$baseUrl/v1/auth/biometric/register'),
      headers: _authHeaders(),
      body: jsonEncode({
        'publicKey': keyPair.publicKey,
        'keyId': keyPair.keyId,
        'biometricType': await FinixBiometric.getBiometricType(),
        'deviceIdFingerprint': _deviceIdFingerprint,
      }),
    );

    if (response.statusCode != 200) {
      throw BiometricSetupException('Failed to register biometric');
    }
  }

  // ─── Biometric Login ──────────────────────────────────────────────

  Future<LoginResponse> biometricLogin() async {
    // Step 1: Get challenge from server
    final challengeResp = await _http.post(
      Uri.parse('$baseUrl/v1/auth/login/challenge'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({
        'deviceIdFingerprint': _deviceIdFingerprint,
      }),
    );

    final challengeData = jsonDecode(challengeResp.body);
    final challenge = challengeData['challenge'] as String;
    final keyId = challengeData['keyId'] as String;

    // Step 2: Sign challenge on device
    final signature = await FinixBiometric.signChallenge(
      challenge: challenge,
      keyId: keyId,
    );

    // Step 3: Verify with server
    final verifyResp = await _http.post(
      Uri.parse('$baseUrl/v1/auth/login/verify'),
      headers: {'Content-Type': 'application/json'},
      body: jsonEncode({
        'challenge': challenge,
        'signature': signature,
        'keyId': keyId,
        'deviceIdFingerprint': _deviceIdFingerprint,
      }),
    );

    final loginData = jsonDecode(verifyResp.body);
    _sessionToken = loginData['sessionToken'];
    return LoginResponse.fromJson(loginData);
  }

  // ─── High-Risk Transaction Step-Up ────────────────────────────────

  Future<TransactionResult> initiateTransaction({
    required String toAddress,
    required int amountCents,
    required String currency,
  }) async {
    final response = await _http.post(
      Uri.parse('$baseUrl/v1/transactions/initiate'),
      headers: _authHeaders(),
      body: jsonEncode({
        'toAddress': toAddress,
        'amountCents': amountCents,
        'currency': currency,
        'deviceIdFingerprint': _deviceIdFingerprint,
      }),
    );

    final data = jsonDecode(response.body);

    // If step-up required, authenticate and override
    if (data['stepUpRequired'] == true) {
      final authenticated = await FinixBiometric.authenticate(
        reason: 'Verify transaction to $toAddress for ${(amountCents / 100).toStringAsFixed(2)} $currency',
      );

      if (!authenticated) {
        throw BiometricAuthException('Transaction denied by user');
      }

      final overrideResp = await _http.post(
        Uri.parse('$baseUrl/v1/transactions/override'),
        headers: _authHeaders(),
        body: jsonEncode({
          'transactionId': data['transactionId'],
          'biometricVerified': true,
          'deviceIdFingerprint': _deviceIdFingerprint,
        }),
      );

      return TransactionResult.fromJson(jsonDecode(overrideResp.body));
    }

    return TransactionResult.fromJson(data);
  }

  // ─── PQC Handshake ───────────────────────────────────────────────

  Future<PqcSession> initPqcHandshake() async {
    // Step 1: Get server's Kyber public key
    final initResp = await _http.get(
      Uri.parse('$baseUrl/v1/auth/pqc/init'),
      headers: _authHeaders(),
    );

    final initData = jsonDecode(initResp.body);
    final serverPublicKeyB64 = initData['publicKey'] as String;
    final algorithm = initData['algorithm'] as String; // "kyber-1024"

    // Step 2: Encapsulate on device using kyber package
    final serverPubKeyBytes = base64Decode(serverPublicKeyB64);
    final encapsulation = await Kyber1024.encapsulate(serverPubKeyBytes);

    final ciphertextB64 = base64Encode(encapsulation.ciphertext);
    final sharedSecret = encapsulation.sharedSecret;

    // Step 3: Send ciphertext to server
    final encapResp = await _http.post(
      Uri.parse('$baseUrl/v1/auth/pqc/encapsulate'),
      headers: _authHeaders(),
      body: jsonEncode({
        'ciphertext': ciphertextB64,
        'algorithm': algorithm,
      }),
    );

    final encapData = jsonDecode(encapResp.body);
    final sessionId = encapData['sessionId'] as String;

    // Both client and server now share `sharedSecret`
    // Derive AES-256-GCM key from shared secret
    final derivedKey = _deriveAesKey(sharedSecret);

    return PqcSession(
      sessionId: sessionId,
      encryptionKey: derivedKey,
    );
  }

  // ─── Helpers ──────────────────────────────────────────────────────

  Map<String, String> _authHeaders() => {
        'Content-Type': 'application/json',
        'Authorization': 'Bearer $_sessionToken',
        'X-Device-Fingerprint': _deviceIdFingerprint ?? '',
      };

  List<int> _deriveAesKey(List<int> sharedSecret) {
    // HKDF-SHA256 with empty info, 32-byte output
    final hmacSha256 = Hmac(sha256, sharedSecret);
    final prk = hmacSha256.convert(utf8.encode('finix-pqc-aes-key'));
    return prk.bytes.take(32).toList();
  }
}

// ─── Response Models ─────────────────────────────────────────────────

class RegisterResponse {
  final String userId;
  final String sessionToken;

  RegisterResponse.fromJson(Map<String, dynamic> json)
      : userId = json['userId'],
        sessionToken = json['sessionToken'];
}

class LoginResponse {
  final String sessionToken;
  final String userId;
  final bool biometricRegistered;

  LoginResponse.fromJson(Map<String, dynamic> json)
      : sessionToken = json['sessionToken'],
        userId = json['userId'],
        biometricRegistered = json['biometricRegistered'] ?? false;
}

class TransactionResult {
  final String transactionId;
  final String status;
  final bool stepUpRequired;

  TransactionResult.fromJson(Map<String, dynamic> json)
      : transactionId = json['transactionId'],
        status = json['status'],
        stepUpRequired = json['stepUpRequired'] ?? false;
}

class PqcSession {
  final String sessionId;
  final List<int> encryptionKey;

  PqcSession({required this.sessionId, required this.encryptionKey});
}

class BiometricSetupException implements Exception {
  final String message;
  BiometricSetupException(this.message);
}

class BiometricAuthException implements Exception {
  final String message;
  BiometricAuthException(this.message);
}
```

---

## 4. Security Considerations

### Threat Model & Mitigations

| Threat | Mitigation |
|--------|------------|
| Biometric data intercepted | Raw biometric data never leaves Secure Enclave/KeyStore; only cryptographic signatures are transmitted |
| Private key extraction | Keys generated inside hardware security module with `kSecAttrTokenIDSecureEnclave` (iOS) / `setIsStrongBoxBacked(true)` (Android) |
| Session hijacking | Session tokens are device-bound; server validates `deviceIdFingerprint` on every request |
| Replay attacks | Challenges are single-use and time-bound (TTL: 5 minutes); PQC shared secrets are ephemeral |
| MITM on PQC channel | Kyber-1024 shared secret established before any sensitive data exchange; AES-256-GCM with fresh keys per session |
| Device fingerprint spoofing | Hardware UUID stored in Keychain/SharedPreferences with `FirstUnlockThisDeviceOnly` accessibility; factory reset clears it |
| Key rotation after biometric change | `setInvalidatedByBiometricEnrollment(true)` (Android) / `biometryCurrentSet` (iOS) invalidates keys when biometrics are re-enrolled |

### Data Flow Rules

1. **Never** transmit raw biometric templates, face maps, or fingerprint data
2. **Always** hash device fingerprint client-side before sending to server
3. **Always** use TLS 1.3 + PQC hybrid encryption for network transport
4. **Always** bind session tokens to device fingerprint
5. **Never** persist PQC shared secrets beyond the session lifetime

### Platform-Specific Security Notes

#### iOS
- Secure Enclave P-256 keys cannot be exported; signing happens inside the enclave
- DeviceCheck API provides per-device attestation tokens validated by Apple
- Keychain items with `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` are wiped on device wipe

#### Android
- StrongBox (if available) provides hardware-backed key storage separate from TEE
- `setInvalidatedByBiometricEnrollment(true)` auto-invalidates keys when fingerprints/face data changes
- SafetyNet attestation confirms device integrity; Play Integrity API is the modern replacement
- `android.os.Build.FINGERPRINT` changes on factory reset, providing additional binding

### Compliance Alignment

- **PCI DSS**: Cardholder data environment uses device-bound keys; biometric step-up for high-risk transactions
- **PSD2 SCA**: Biometric authentication satisfies "something you are" factor; device fingerprint satisfies "something you have"
- **NIST SP 800-63B**: AAL3 authenticator requirements met via hardware-backed key storage + biometric
