import importlib.util
from pathlib import Path


def test_quick_start_embedded_jpeg_is_valid() -> None:
    example = Path(__file__).parents[1] / "examples" / "quick_start.py"
    spec = importlib.util.spec_from_file_location("quick_start_example", example)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)

    assert module.TEST_JPEG.startswith(b"\xff\xd8")
    assert module.TEST_JPEG.endswith(b"\xff\xd9")
