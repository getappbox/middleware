import Vapor

/// Validates S3 install-page targets without requesting their public metadata.
enum PublicInstallLink {
    static func validate(_ value: String, base: String) throws -> String {
        guard isPublicHTTPSURL(value), isPublicHTTPSURL(base),
              let candidate = URLComponents(string: value), let configured = URLComponents(string: base),
              configured.query == nil,
              candidate.host?.lowercased() == configured.host?.lowercased(),
              (candidate.port ?? 443) == (configured.port ?? 443),
              candidate.percentEncodedPath == basePath(configured.percentEncodedPath) + "/"
                || candidate.percentEncodedPath == configured.percentEncodedPath,
              let query = candidate.percentEncodedQuery,
              let parameters = parameters(query), Set(parameters.keys) == ["provider", "url"],
              parameters["provider"] == "s3", let metadata = parameters["url"],
              isPublicHTTPSURL(metadata), URLComponents(string: metadata)?.percentEncodedPath.hasSuffix("/appinfo.json") == true else {
            throw APIError(status: .badRequest, code: "invalid_url", message: "Expected an S3 install-page URL with a public HTTPS appinfo.json source.")
        }
        return value
    }

    private static func basePath(_ path: String) -> String {
        var result = path
        while result.hasSuffix("/") { result.removeLast() }
        return result
    }

    private static func parameters(_ query: String) -> [String: String]? {
        var result: [String: String] = [:]
        for pair in query.split(separator: "&", omittingEmptySubsequences: false) {
            let parts = pair.split(separator: "=", maxSplits: 1, omittingEmptySubsequences: false)
            guard parts.count == 2,
                  let name = String(parts[0]).replacingOccurrences(of: "+", with: " ").removingPercentEncoding,
                  let value = String(parts[1]).replacingOccurrences(of: "+", with: " ").removingPercentEncoding,
                  result[name] == nil else { return nil }
            result[name] = value
        }
        return result
    }

    private static func isPublicHTTPSURL(_ value: String) -> Bool {
        guard value.utf8.count <= 8192, !value.contains("\\"), validPercentEncoding(value),
              !value.unicodeScalars.contains(where: { CharacterSet.whitespacesAndNewlines.contains($0) || CharacterSet.controlCharacters.contains($0) }),
              let components = URLComponents(string: value), components.scheme?.lowercased() == "https",
              let host = components.host, !host.isEmpty,
              components.user == nil, components.password == nil, components.fragment == nil,
              !(components.queryItems ?? []).contains(where: { signingQueryNames.contains($0.name.lowercased()) }) else { return false }
        return true
    }

    private static func validPercentEncoding(_ value: String) -> Bool {
        let bytes = Array(value.utf8)
        var index = 0
        while index < bytes.count {
            if bytes[index] == 37 {
                guard index + 2 < bytes.count, isHex(bytes[index + 1]), isHex(bytes[index + 2]) else { return false }
                index += 3
            } else { index += 1 }
        }
        return true
    }

    private static let signingQueryNames: Set<String> = ["x-amz-signature", "x-amz-credential", "x-amz-security-token", "awsaccesskeyid", "signature", "key-pair-id"]

    private static func isHex(_ byte: UInt8) -> Bool {
        (48...57).contains(byte) || (65...70).contains(byte) || (97...102).contains(byte)
    }
}
