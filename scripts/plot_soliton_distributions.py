#!/usr/bin/env python3
"""Plot ideal and robust soliton degree distributions.

The script uses the LT-code notation from the SeF paper:

    rho(d) = ideal soliton distribution
    mu(d)  = robust soliton distribution

By default it writes the figure to images/tests/soliton_distributions.png.
"""

from __future__ import annotations

import argparse
import math
import os
from pathlib import Path

os.environ.setdefault("MPLCONFIGDIR", "/private/tmp/matplotlib")
os.environ.setdefault("XDG_CACHE_HOME", "/private/tmp/fontconfig")


def ideal_soliton(k: int) -> list[float]:
    """Return rho(d) for d = 1, ..., k."""
    rho = [0.0] * k
    rho[0] = 1.0 / k

    for d in range(2, k + 1):
        rho[d - 1] = 1.0 / (d * (d - 1))

    return rho


def robust_soliton(k: int, c: float, delta: float) -> tuple[list[float], float]:
    """Return mu(d) for d = 1, ..., k and the corresponding R value."""
    rho = ideal_soliton(k)
    r = c * math.sqrt(k) * math.log(k / delta)

    if r <= 0:
        raise ValueError("R must be positive; check c, k, and delta.")

    # The paper indexes the spike at k/R. In code we need an integer degree.
    spike_degree = max(1, min(k, int(k / r)))

    theta = [0.0] * k
    for d in range(1, spike_degree):
        theta[d - 1] = r / (d * k)

    theta[spike_degree - 1] = (r / k) * math.log(r / delta)

    beta = sum(rho[d] + theta[d] for d in range(k))
    mu = [(rho[d] + theta[d]) / beta for d in range(k)]

    return mu, r


def plot_distributions(k: int, c: float, delta: float, output: Path) -> None:
    """Generate and save a plot comparing both distributions."""
    import matplotlib
    matplotlib.use("Agg")
    import matplotlib.pyplot as plt

    degrees = list(range(1, k + 1))
    rho = ideal_soliton(k)
    mu, r = robust_soliton(k, c, delta)

    fig, axes = plt.subplots(1, 2, figsize=(14, 5), constrained_layout=True)

    axes[0].bar(degrees, rho, color="#2563eb", width=0.9)
    axes[0].set_title("Ideal Soliton Distribution")
    axes[0].set_xlabel("Degree d")
    axes[0].set_ylabel("Probability")
    axes[0].grid(True, alpha=0.25)

    axes[1].bar(degrees, mu, color="#dc2626", width=0.9)
    axes[1].set_title("Robust Soliton Distribution (c = 0.1; delta = 0.01)")
    axes[1].set_xlabel("Degree d")
    axes[1].set_ylabel("Probability")
    axes[1].grid(True, alpha=0.25)

    fig.suptitle(f"Soliton degree distributions (k={k}, c={c}, delta={delta}, R={r:.2f})")

    output.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(output, dpi=200)
    plt.close(fig)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Plot ideal and robust soliton distributions used by LT/Fountain codes."
    )
    parser.add_argument("--k", type=int, default=10, help="Number of source blocks in an epoch.")
    parser.add_argument("--c", type=float, default=0.03, help="Robust soliton c parameter.")
    parser.add_argument("--delta", type=float, default=0.01, help="Target decoding failure probability.")
    parser.add_argument(
        "--output",
        type=Path,
        default=Path("images/tests/soliton_distributions.png"),
        help="Path where the PNG plot should be written.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()

    if args.k < 2:
        raise ValueError("k must be at least 2.")
    if args.c <= 0:
        raise ValueError("c must be positive.")
    if not 0 < args.delta < 1:
        raise ValueError("delta must be between 0 and 1.")

    plot_distributions(args.k, args.c, args.delta, args.output)
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()
