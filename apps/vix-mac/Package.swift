// swift-tools-version:6.0
import PackageDescription

let package = Package(
    name: "vix-mac",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .library(name: "VixProtocol", targets: ["VixProtocol"]),
        .library(name: "VixClient", targets: ["VixClient"]),
        .executable(name: "vix-mac-probe", targets: ["vix-mac-probe"]),
    ],
    targets: [
        // Generated wire-protocol models + the hand-written JSONValue support
        // type. VixProtocol/Generated.swift is produced by `make mac-models`.
        .target(name: "VixProtocol"),

        // Unix-socket NDJSON transport and the high-level session client.
        .target(name: "VixClient", dependencies: ["VixProtocol"]),

        // Headless CLI proof: open a session, stream a turn, answer a confirm.
        .executableTarget(name: "vix-mac-probe", dependencies: ["VixClient"]),

        .testTarget(name: "VixProtocolTests", dependencies: ["VixClient"]),
    ]
)
