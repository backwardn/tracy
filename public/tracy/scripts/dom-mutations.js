(() => {
  // This observer will be used to observe changes in the DOM. It will batches
  // DOM changes and send them to the API/ server if it finds a tracer string.
  const observer = new MutationObserver(mutations => {
    let parentNode = null;

    for (let mutation of mutations) {
      if (mutation.addedNodes.length > 0) {
        mutation.addedNodes.forEach(node => {
          // Ignore scripts injected from the background page.
          if (
            node.src &&
            (node.src.startsWith("moz-extension") ||
              node.src.startsWith("chrome-extension"))
          ) {
            return;
          }
          // Check to see if a node is a child of the parentNode if so don't add
          // it because we already have that data
          if (
            (parentNode === null || !parentNode.contains(node)) &&
            // Ignore the dropdown that is created when you click the owl.
            node.id !== "tag-menu"
          ) {
            // The only supported DOM types that we care about are `DOM` (1) and
            // `text` (3).
            if (node.nodeType === Node.ELEMENT_NODE) {
              // In the case of a DOM type, check all the node's children for
              // input fields. Use this as a chance to restyle new inputs that
              // were not caught earlier.
              parentNode = node;
              retryingSend({
                "message-type": "job",
                type: "dom",
                msg: node.outerHTML,
                location: document.location.href
              });
              highlight.addClickToFill(node, false);
              window.postMessage(
                {
                  "message-type": "dom",
                  type: "form"
                },
                "*"
              );
            } else if (node.nodeType == Node.TEXT_NODE) {
              retryingSend({
                "message-type": "job",
                type: "text",
                msg: node.textContent,
                location: document.location.href
              });
            }
          }
        });
      } else {
        if (mutation.type == "attributes") {
          // Ignore the screenshot class changes and the changes
          // to the style of the own dropdown.
          if (
            mutation.target.classList.contains("screenshot") ||
            mutation.target.classList.contains("screenshot-done") ||
            mutation.target.id == "tag-menu"
          ) {
            return;
          }
          retryingSend({
            "message-type": "job",
            type: "dom",
            msg: mutation.target.outerHTML,
            location: document.location.href
          });
        } else if (mutation.type == "characterData") {
          retryingSend({
            "message-type": "job",
            type: "text",
            msg: mutation.target.nodeValue,
            location: document.location.href
          });
        }
      }
    }
  });

  // The configuration for the observer. We want to pretty much watch for everything.
  const observerConfig = {
    attributes: true,
    childList: true,
    characterData: true,
    characterDataOldValue: true,
    subtree: true
  };

  const retryingSend = async message => {
    for (;;) {
      try {
        return await util.send(message);
      } catch (e) {
        console.log("retrying...", e);
        await new Promise(r => setTimeout(r, 1000));
      }
    }
  };
  observer.observe(document.documentElement, observerConfig);
})();
