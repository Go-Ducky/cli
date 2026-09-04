class Goducky < Formula
  desc "AI coding agent that runs in your terminal (local + API models)"
  homepage "https://github.com/Go-Ducky/goducky-cli"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/Go-Ducky/goducky-cli/releases/download/v0.1.0/goducky-darwin-arm64"
      sha256 "REPLACE_WITH_DARWIN_ARM64_SHA256"
    else
      url "https://github.com/Go-Ducky/goducky-cli/releases/download/v0.1.0/goducky-darwin-amd64"
      sha256 "REPLACE_WITH_DARWIN_AMD64_SHA256"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/Go-Ducky/goducky-cli/releases/download/v0.1.0/goducky-linux-arm64"
      sha256 "REPLACE_WITH_LINUX_ARM64_SHA256"
    else
      url "https://github.com/Go-Ducky/goducky-cli/releases/download/v0.1.0/goducky-linux-amd64"
      sha256 "REPLACE_WITH_LINUX_AMD64_SHA256"
    end
  end

  def install
    bin.install Dir["goducky-darwin*", "goducky-linux*"].first => "goducky"
  end

  test do
    assert_match "goducky", shell_output("#{bin}/goducky --version")
  end
end
